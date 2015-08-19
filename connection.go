package mudoo

import (
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
    fd               uint32
    nc               *net.TCPConn
    raddr            string
    online           bool
    lastConnected    int64
    lastDisconnected int64
    numHeartbeats    int
    numConns         int       // Total number of reconnects.
    wakeupFlusher    chan byte // Used internally to wake up the flusher.
    wakeupReader     chan byte // Used internally to wake up the reader.
    decBuf           *Buffer
}

// NewConn creates a new connection for the sio. It generates the session id and
// prepares the internal structure for usage.
func newConn(serv *Server, fd uint32, nc *net.TCPConn) (c *Conn, err error) {
    host, _, err := net.SplitHostPort(nc.RemoteAddr().String())
    if err != nil {
        serv.Log("mudoo/newConn: GetRemoteAddr:", err)
        return
    }

    c = &Conn{
        serv:          serv,
        fd:            fd,
        nc:            nc,
        raddr:         host,
        online:        true,
        lastConnected: time.Now().UnixNano(),
        wakeupFlusher: make(chan byte),
        wakeupReader:  make(chan byte),
        numConns:      0,
        numHeartbeats: 0,
        decBuf:        new(Buffer),
    }

    nc.SetReadBuffer(serv.config.ReadBufferSize)
    nc.SetWriteBuffer(serv.config.WriteBufferSize)

    go c.keepalive()
    // go c.flusher()
    go c.reader()

    return
}

// String returns a string representation of the connection and implements the
// fmt.Stringer interface.
func (c *Conn) String() string {
    return fmt.Sprintf("(%d)[%s]", c.fd, c.raddr)
}

// RemoteAddr returns the remote network address of the connection in IP:port format
func (c *Conn) RemoteAddr() string {
    return c.raddr
}

// Send queues data for a delivery. It is totally content agnostic with one exception:
// the given data must be marshallable by the standard protobuf package. If the queue to send
// has reached sio.config.QueueLength or the connection has been disconnected,
// then the data is dropped and an error is returned.
func (c *Conn) Send(data Message) error {
    c.mutex.Lock()
    defer c.mutex.Unlock()

    if !c.online {
        return ErrNotConnected
    }

    return c.serv.codec.Send(c, data)
}

func (c *Conn) Close() error {
    c.mutex.Lock()

    if !c.online {
        c.mutex.Unlock()
        return ErrNotConnected
    }

    c.disconnect()
    c.mutex.Unlock()

    c.serv.doDisconnect(c)
    return nil
}

func (c *Conn) disconnect() {
    c.serv.Log("mudoo/conn: disconnected:", c)
    c.nc.Close()
    c.online = false
    close(c.wakeupFlusher)
    close(c.wakeupReader)
}

// Receive decodes and handles data received from the socket.
// It uses c.sio.codec to decode the data. The received non-heartbeat
// messages (frames) are then passed to c.sio.onMessage method and the
// heartbeats are processed right away (TODO).
func (c *Conn) receive(data []byte) {
    c.decBuf.WriteRawBytes(data)
    c.serv.config.Codec.OnMessage(c, c.decBuf, time.Now().UnixNano())
}

func (c *Conn) keepalive() {
    //     c.ticker = time.NewTicker(time.Duration(c.serv.config.HeartbeatInterval) * time.Second)
    //     defer c.ticker.Stop()

    // Loop:
    //     for t := range c.ticker.C {
    //         c.mutex.Lock()

    //         if !c.online {
    //             c.mutex.Unlock()
    //             return
    //         }

    //         if (!c.online && (time.Now().UnixNano()-c.lastDisconnected > c.serv.config.ReconnectTimeout)) || int(c.lastHeartbeat) < c.numHeartbeats {
    //             c.disconnect()
    //             c.mutex.Unlock()
    //             break
    //         }

    //         c.numHeartbeats++

    //         select {
    //         case c.queue <- heartbeat(c.numHeartbeats):
    //         default:
    //             c.serv.Log("mudoo/keepalive: unable to queue heartbeat. fail now. TODO: FIXME", c)
    //             c.disconnect()
    //             c.mutex.Unlock()
    //             break Loop
    //         }

    //         c.mutex.Unlock()
    //     }

    //     c.serv.doDisconnect(c)
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
// func (c *Conn) flusher() {
//     buf := new(bytes.Buffer)
//     var err error
//     var msg interface{}
//     var n int

//     for msg = range c.queue {
//         buf.Reset()
//         err = c.enc.Encode(buf, msg)
//         n = 1

//         if err == nil {

//         DrainLoop:
//             for n < c.serv.config.QueueLength {
//                 select {
//                 case msg = <-c.queue:
//                     n++
//                     if err = c.enc.Encode(buf, msg); err != nil {
//                         break DrainLoop
//                     }

//                 default:
//                     break DrainLoop
//                 }
//             }
//         }
//         if err != nil {
//             c.serv.Logf("mudoo/conn: flusher/encode: lost %d messages (%d bytes): %s %s", n, buf.Len(), err, c)
//             continue
//         }

//     FlushLoop:
//         for {
//             for {
//                 c.mutex.Lock()
//                 _, err = buf.WriteTo(c.nc)
//                 c.mutex.Unlock()

//                 if err == nil {
//                     break FlushLoop
//                 }
//             }

//             if _, ok := <-c.wakeupFlusher; !ok {
//                 return
//             }
//         }
//     }
// }

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
