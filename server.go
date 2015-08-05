package mudoo

import (
    "net"
    "sync"
)

type Server struct {
    config       Config              // Holds the configuration values.
    addr         string              // Listen on port.
    listener     *net.Listener       // Holds the listener.
    sessions     map[SessionID]*Conn // Holds the outstanding conns.
    sessionsLock *sync.RWMutex       // Protects the conns.

    callbacks struct {
        onConnect    func(*Conn)          // Invoked on new connection.
        onDisconnect func(*Conn)          // Invoked on a lost connection.
        onMessage    func(*Conn, Message) // Invoked on a message.
    }
}

// NewServer creates a new socket server with chosen transports and configuration
// options. If transports is nil, the DefaultTransports is used. If config is nil,
// the DefaultConfig is used.
func NewServer(config *Config) *Server {
    tcpAddr, err := net.ResolveTCPAddr("tcp4", config.ListenAddr)
    if err != nil {
        return nil
    }

    if config == nil {
        config = &DefaultConfig
    }

    serv := &Server{
        config:       *config,
        addr:         tcpAddr,
        sessions:     make(map[SessionID]*Conn),
        sessionsLock: new(sync.RWMutex),
    }

    return serv
}

func (serv *Server) Run() {
    ln, err := net.ListenTCP("tcp", serv.addr)
    if err != nil {
        serv.Log("mudoo/Run: bind tcp port failure:", err)
        return
    }

    var tempDelay time.Duration // how long to sleep on accept failure
    for {
        co, err := listener.Accept()
        if err != nil {
            if ne, ok := err.(net.Error); ok && ne.Temporary() {
                if tempDelay == 0 {
                    tempDelay = 5 * time.Millisecond
                } else {
                    tempDelay *= 2
                }
                if max := 1 * time.Second; tempDelay > max {
                    tempDelay = max
                }
                log.Printf("%v: Accept error: %v; retrying in %v", name, err, tempDelay)
                time.Sleep(tempDelay)
                continue
            }
            return err
        }
        tempDelay = 0
        c, err := newConn(serv, co)
        if err != nil {
            return
        }
        serv.doSocketConnect(conn)
    }
}

// Broadcast schedules data to be sent to each connection.
func (serv *Server) Broadcast(data interface{}) {
    serv.BroadcastExcept(nil, data)
}

// BroadcastExcept schedules data to be sent to each connection except
// c. It does not care about the type of data, but it must marshallable
// by the standard json-package.
func (serv *Server) BroadcastExcept(c *Conn, data interface{}) {
    serv.sessionsLock.RLock()
    defer serv.sessionsLock.RUnlock()

    for _, v := range serv.sessions {
        if v != c {
            v.Send(data)
        }
    }
}

// GetConn digs for a conn with fd and returns it.
func (serv *Server) GetConn(sessid SessionID) (c *Conn) {
    serv.sessionsLock.RLock()
    c = serv.sessions[sessid]
    serv.sessionsLock.RUnlock()
    return
}

// OnConnect sets f to be invoked when a new connection is established. It passes
// the established connection as an argument to the callback.
func (serv *Server) OnConnect(f func(*Conn)) os.Error {
    serv.callbacks.onConnect = f
    return nil
}

// OnDisconnect sets f to be invoked when a connection is considered to be lost.
// It passes the established connection as an argument to the callback. After
// disconnection the connection is considered to be destroyed, and it should not
// be used anymore.
func (serv *Server) OnDisconnect(f func(*Conn)) os.Error {
    serv.callbacks.onDisconnect = f
    return nil
}

// OnMessage sets f to be invoked when a message arrives. It passes the
// established connection along with the received message as arguments
// to the callback.
func (serv *Server) OnMessage(f func(*Conn, Message)) os.Error {
    serv.callbacks.OnMessage = f
    return nil
}

func (serv *Server) Log(v ...interface{}) {
    if logger := serv.config.Logger; logger != nil {
        logger.Println(v...)
    }
}

func (serv *Server) Logf(format string, v ...interface{}) {
    if logger := serv.config.Logger; logger != nil {
        logger.Printf(format, v...)
    }
}

// OnConnect is invoked by a connection when a new connection has been
// established successfully. The establised connection is passed as an
// argument. It stores the connection and calls the user's OnConnect callback.
func (serv *Server) doSocketConnect(c *Conn) {
    serv.sessionsLock.Lock()
    serv.sessions[c.sessid] = c
    serv.sessionsLock.Unlock()

    if fn := serv.callbacks.onConnect; fn != nil {
        fn(c)
    }
}

// OnDisconnect is invoked by a connection when the connection is considered
// to be lost. It removes the connection and calls the user's OnDisconnect callback.
func (serv *Server) doSocketDisconnect(c *Conn) {
    serv.sessionsLock.Lock()
    serv.sessions[c.sessid] = nil, false
    serv.sessionsLock.Unlock()

    if fn := serv.callbacks.onDisconnect; fn != nil {
        fn(c)
    }
}

// OnMessage is invoked by a connection when a new message arrives. It passes
// this message to the user's OnMessage callback.
func (serv *Server) doMessageReceived(c *Conn, msg Message) {
    if fn := serv.callbacks.onMessage; fn != nil {
        fn(c, msg)
    }
}
