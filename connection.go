package mudoo

import (
    "bytes"
    "errors"
    "fmt"
    "net"
    "sync"
    "time"
)

var (
    // ErrDestroyed is used when the connection has been disconnected (i.e. can't be used anymore).
    ErrDestroyed = errors.New("connection is disconnected")

    // ErrQueueFull is used when the send queue is full.
    ErrQueueFull = errors.New("send queue is full")

    // ErrNotConnected is used when some action required the connection to be online,
    // but it wasn't.
    ErrNotConnected = errors.New("not connected")

    // ErrConnected is used when some action required the connection to be offline,
    // but it wasn't.
    ErrConnected = errors.New("already connected")

    emptyResponse = []byte{}
)

// Conn represents a single session and handles its handshaking,
// message buffering and reconnections.
type Conn struct {
    mutex            sync.Mutex
    serv             *Server
    nc               *net.TCPConn
    sessid           SessionID
    online           bool
    lastConnected    int64
    lastDisconnected int64
    lastHeartbeat    heartbeat
    numHeartbeats    int
    ticker           *time.Ticker
    queue            chan interface{} // Buffers the outgoing messages.
    numConns         int              // Total number of reconnects.
    handshaked       bool             // Indicates if the handshake has been sent.
    disconnected     bool             // Indicates if the connection has been disconnected.
    wakeupFlusher    chan byte        // Used internally to wake up the flusher.
    wakeupReader     chan byte        // Used internally to wake up the reader.
    enc              Encoder
    dec              Decoder
    decBuf           bytes.Buffer
    raddr            string
}

// NewConn creates a new connection for the sio. It generates the session id and
// prepares the internal structure for usage.
func newConn(serv *Server, nc *net.TCPConn) (c *Conn, err error) {
    var sessid SessionID
    if sessid, err = NewSessionID(); err != nil {
        serv.Log("mudoo/newConn: NewSessionID:", err)
        return
    }

    host, port, err := net.SplitHostPort(nc.RemoteAddr().String())
    if err != nil {
        serv.Log("mudoo/newConn: GetRemoteAddr:", err)
        return
    }

    c = &Conn{
        serv:          serv,
        nc:            nc,
        sessid:        sessid,
        online:        true,
        lastConnected: time.Now().UnixNano(),
        wakeupFlusher: make(chan byte),
        wakeupReader:  make(chan byte),
        queue:         make(chan interface{}, serv.config.QueueLength),
        numConns:      0,
        enc:           serv.config.Codec.NewEncoder(),
        raddr:         host,
    }

    nc.SetReadBuffer(serv.config.ReadBufferSize)
    nc.SetWriteBuffer(serv.config.WriteBufferSize)
    c.dec = serv.config.Codec.NewDecoder(&c.decBuf)

    go c.keepalive()
    go c.flusher()
    go c.reader()

    return
}

// String returns a string representation of the connection and implements the
// fmt.Stringer interface.
func (c *Conn) String() string {
    return fmt.Sprintf("%v[%v]", c.sessid, c.nc)
}

// RemoteAddr returns the remote network address of the connection in IP:port format
func (c *Conn) RemoteAddr() string {
    return c.raddr
}

// Send queues data for a delivery. It is totally content agnostic with one exception:
// the given data must be one of the following: a handshake, a heartbeat, an int, a string or
// it must be otherwise marshallable by the standard json package. If the send queue
// has reached sio.config.QueueLength or the connection has been disconnected,
// then the data is dropped and a an error is returned.
func (c *Conn) Send(data interface{}) (err error) {
    c.mutex.Lock()
    defer c.mutex.Unlock()

    if c.disconnected {
        return ErrDestroyed
    }

    select {
    case c.queue <- data:
    default:
        return ErrQueueFull
    }

    return nil
}

func (c *Conn) Close() error {
    c.mutex.Lock()

    if c.disconnected {
        c.mutex.Unlock()
        return ErrNotConnected
    }

    c.disconnect()
    c.mutex.Unlock()

    c.serv.doDisconnect(c)
    return nil
}

// Handshake sends the handshake to the socket.
func (c *Conn) handshake() error {
    return c.enc.Encode(c.nc, handshake(c.sessid))
}

func (c *Conn) disconnect() {
    c.serv.Log("mudoo/conn: disconnected:", c)
    c.nc.Close()
    c.disconnected = true
    close(c.wakeupFlusher)
    close(c.wakeupReader)
    close(c.queue)
}

// Receive decodes and handles data received from the socket.
// It uses c.sio.codec to decode the data. The received non-heartbeat
// messages (frames) are then passed to c.sio.onMessage method and the
// heartbeats are processed right away (TODO).
func (c *Conn) receive(data []byte) {
    c.decBuf.Write(data)
    msgs, err := c.dec.Decode()
    if err != nil {
        c.serv.Log("mudoo/conn: receive/decode:", err, c)
        return
    }

    for _, m := range msgs {
        if hb, ok := m.heartbeat(); ok {
            c.lastHeartbeat = hb
        } else {
            c.serv.doMessageReceived(c, m)
        }
    }
}

func (c *Conn) keepalive() {
    c.ticker = time.NewTicker(time.Duration(c.serv.config.HeartbeatInterval) * time.Second)
    defer c.ticker.Stop()

Loop:
    for t := range c.ticker.C {
        c.mutex.Lock()

        if c.disconnected {
            c.mutex.Unlock()
            return
        }

        if (!c.online && (time.Now().UnixNano()-c.lastDisconnected > c.serv.config.ReconnectTimeout)) || int(c.lastHeartbeat) < c.numHeartbeats {
            c.disconnect()
            c.mutex.Unlock()
            break
        }

        c.numHeartbeats++

        select {
        case c.queue <- heartbeat(c.numHeartbeats):
        default:
            c.serv.Log("mudoo/keepalive: unable to queue heartbeat. fail now. TODO: FIXME", c)
            c.disconnect()
            c.mutex.Unlock()
            break Loop
        }

        c.mutex.Unlock()
    }

    c.serv.doDisconnect(c)
}

// Flusher waits for messages on the queue. It then
// tries to write the messages to the underlaying socket and
// will keep on trying until the wakeupFlusher is killed or the payload
// can be delivered. It is responsible for persisting messages until they
// can be succesfully delivered. No more than c.sio.config.QueueLength messages
// should ever be waiting for a delivery.
//
// NOTE: the c.sio.config.QueueLength is not a "hard limit", because one could have
// max amount of messages waiting in the queue and in the payload itself
// simultaneously.
func (c *Conn) flusher() {
    buf := new(bytes.Buffer)
    var err error
    var msg interface{}
    var n int

    for msg = range c.queue {
        buf.Reset()
        err = c.enc.Encode(buf, msg)
        n = 1

        if err == nil {

        DrainLoop:
            for n < c.serv.config.QueueLength {
                select {
                case msg = <-c.queue:
                    n++
                    if err = c.enc.Encode(buf, msg); err != nil {
                        break DrainLoop
                    }

                default:
                    break DrainLoop
                }
            }
        }
        if err != nil {
            c.serv.Logf("mudoo/conn: flusher/encode: lost %d messages (%d bytes): %s %s", n, buf.Len(), err, c)
            continue
        }

    FlushLoop:
        for {
            for {
                c.mutex.Lock()
                _, err = buf.WriteTo(c.nc)
                c.mutex.Unlock()

                if err == nil {
                    break FlushLoop
                }
            }

            if _, ok := <-c.wakeupFlusher; !ok {
                return
            }
        }
    }
}

// Reader reads from the c.socket until the c.wakeupReader is closed.
// It is responsible for detecting unrecoverable read errors and timeouting
// the connection. When a read fails previously mentioned reasons, it will
// call the c.disconnect method and start waiting for the next event on the
// c.wakeupReader channel.
func (c *Conn) reader() {
    buf := make([]byte, c.serv.config.ReadBufferSize)

    for {
        c.mutex.Lock()
        socket := c.nc
        c.mutex.Unlock()

        for {
            nr, err := c.nc.Read(buf)
            if err != nil {
                if neterr, ok := err.(*net.OpError); ok && neterr.Timeout() {
                    c.serv.Log("mudoo/conn: lost connection (timeout):", c)
                    socket.Write(emptyResponse)
                } else {
                    c.serv.Log("mudoo/conn: lost connection:", c)
                }
                break
            } else if nr < 0 {
                break
            } else if nr > 0 {
                c.receive(buf[0:nr])
            }
        }

        c.mutex.Lock()
        c.lastDisconnected = time.Now().UnixNano()
        socket.Close()
        if c.nc == socket {
            c.online = false
        }
        c.mutex.Unlock()

        if _, ok := <-c.wakeupReader; !ok {
            break
        }
    }
}
