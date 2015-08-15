package mudoo

import (
    "io"
    "net"

    "github.com/golang/protobuf/proto"
)

type IClient interface {
    io.Closer

    Dial(string, string) error
    OnConnect(func())
    OnDisconnect(func())
    Send(Message) error
    OnMessage(func(Message))
}

type Client struct {
    connected    bool
    nc           *net.TCPConn
    decBuf       *Buffer
    onConnect    func()
    onDisconnect func()
    prototypes   map[uint16]proto.Message
    callbacks    map[uint16]func(proto.Message)
}

func NewClient() *Client {
    c := &Client{
        connected:  false,
        decBuf:     new(Buffer),
        prototypes: make(map[uint16]proto.Message),
        callbacks:  make(map[uint16]func(proto.Message)),
    }
    return c
}

func (c *Client) Dial(listenAddr string) {
    if c.connected {
        return
    }

    tcpAddr, err := net.ResolveTCPAddr("tcp4", listenAddr)
    if err != nil {
        return
    }

    conn, err := net.DialTCP("tcp", nil, tcpAddr)
    if err != nil {
        return
    }

    c.nc = conn
    c.connected = true

    c.reader()
}

func (c *Client) reader() {
    var err error
    var nr int
    var buf []byte

    defer c.Close()

    for {
        if nr, err = c.nc.Read(buf); err != nil {
            return
        }

        if nr > 0 {
            c.decBuf.WriteRawBytes(buf[0:nr])
            // c.codec.OnMessage(c, c.decBuf, time.Now().UnixNano())

            reader := c.decBuf
            pid, err := reader.ReadU16()
            if err != nil {
                return
            }

            var exists bool
            var callback func(proto.Message)
            if callback, exists = c.callbacks[pid]; !exists {
                return
            }

            prototype := c.prototypes[pid]
            if prototype != nil {
                raw, err := reader.ReadRawBytes()
                if err != nil {
                    return
                }

                clone := proto.Clone(prototype)
                err = proto.Unmarshal(raw, clone)
                if err != nil {
                    return
                }

                callback(clone)
            } else {
                callback(nil)
            }
        }
    }
}

// 注册连接打开回调
func (c *Client) OnConnect(fn func()) {
    c.onConnect = fn
}

// 注册连接断开回调
func (c *Client) OnDisconnect(fn func()) {
    c.onDisconnect = fn
}

// 注册接收消息回调
func (c *Client) OnMessage(protoid uint16, prototype proto.Message, callback func(proto.Message)) {
    if prototype != nil {
        c.prototypes[protoid] = prototype

        if callback != nil {
            c.callbacks[protoid] = callback
        }
    }
}

// 发送消息
func (c *Client) Send(msg Message) error {
    if c.nc == nil {
        return ErrNotConnected
    }

    raw, err := proto.Marshal(msg.Body)
    if err != nil {
        return err
    }

    var size int = len(raw)

    writer := Writer()
    writer.WriteU16(uint16(size + 2))
    writer.WriteU16(msg.ProtoID)
    if size > 0 {
        writer.WriteRawBytes(raw)
    }

    _, err = c.nc.Write(writer.Data())
    if err != nil {
        return err
    }

    return nil
}

// 关闭连接
func (c *Client) Close() error {
    if !c.connected {
        return ErrNotConnected
    }
    c.connected = false

    if c.onDisconnect != nil {
        c.onDisconnect()
    }

    return c.nc.Close()
}
