package mudoo

import (
    "github.com/golang/protobuf/proto"
)

// A Codec wraps Encode and Decode methods.
type Codec interface {
    Send(*Conn, Message) error
    OnMessage(*Conn, *Buffer, int64)
    RegisterCallback(uint16, proto.Message, func(*Conn, proto.Message))
}

// --------------------------------------------------------

type GPBCodec struct {
    prototypes map[uint16]proto.Message
    callbacks  map[uint16]func(*Conn, proto.Message)
}

func NewGPBCodec() *GPBCodec {
    codec := new(GPBCodec)
    codec.prototypes = make(map[uint16]proto.Message)
    codec.callbacks = make(map[uint16]func(*Conn, proto.Message))
    return codec
}

func (codec *GPBCodec) Send(conn *Conn, body Message) error {
    raw, err := proto.Marshal(body.Body)
    if err != nil {
        return err
    }

    var size int = len(raw)

    writer := Writer()
    writer.WriteU16(uint16(size + 2))
    writer.WriteU16(body.ProtoID)
    if size > 0 {
        writer.WriteRawBytes(raw)
    }

    _, err = conn.nc.Write(writer.Data())
    if err != nil {
        return err
    }

    return nil
}

func (codec *GPBCodec) RegisterCallback(pid uint16, prototype proto.Message, callback func(*Conn, proto.Message)) {
    if prototype != nil {
        if _, exists := codec.prototypes[pid]; !exists {
            codec.prototypes[pid] = prototype
        }
    }

    if _, exists := codec.callbacks[pid]; !exists {
        codec.callbacks[pid] = callback
    }
}

func (codec *GPBCodec) OnMessage(conn *Conn, buf *Buffer, receiveTime int64) {

    // header
    size, err := buf.ReadU16()
    if err != nil {
        return
    }

    // pid
    pid, err := buf.ReadU16()
    if err != nil {
        return
    }

    var exists bool
    var callback func(*Conn, proto.Message)
    if callback, exists = codec.callbacks[pid]; !exists {
        return
    }

    prototype := codec.prototypes[pid]
    if prototype != nil {
        raw, err := buf.ReadSizedBytes(int(size - 2))
        if err != nil {
            return
        }

        clone := proto.Clone(prototype)
        err = proto.Unmarshal(raw, clone)
        if err != nil {
            return
        }

        callback(conn, clone)
    } else {
        callback(conn, nil)
    }
}
