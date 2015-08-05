package main

import (
    "container/vector"
    "log"
    "sync"

    "github.com/huangqingcheng/mudoo"
)

type Announcement struct {
    Announcement string `json:"announcement"`
}

type Buffer struct {
    Buffer []interface{} `json:"buffer"`
}

type Message Struct {
    Message []string `json:"message"`
}

func main() {
    buffer := new(vector.Vector)
    mutex := new(sync.Mutex)

    config := socketio.DefaultConfig
    serv := mudoo.NewServer(config)

    // when a client connects - send it the buffer and broadcast an annoucement
    serv.OnConnect(func(c *mudoo.Conn) {
        mutex.Lock()
        c.Send(Buffer{buffer.Copy()})
        mutex.Unlock()

        serv.Broadcast(Announcement{"connected: " + c.String()})
    })

    // when a client disconnects - send an announcement
    serv.OnDisconnect(func(c *mudoo.Conn) {
        serv.Broadcast(Announcement{"disconnected: " + c.String()})
    })

    // when a client send a message - broadcast and store it
    serv.OnMessage(func(c *mudoo.Conn, msg mudoo.Message) {
        payload := Message{[]string{c.String(), msg.Data()}}
        mutex.Lock()
        buffer.Push(payload)
        mutex.Unlock()
        serv.Broadcast(payload)
    })

    serv.Run()
}
