package mudoo

import "github.com/golang/protobuf/proto"

type Message struct {
    ProtoID uint16
    Body    proto.Message
}
