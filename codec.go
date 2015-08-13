package mudoo

// A Codec wraps Encode and Decode methods.
type Codec interface {
    Send(*Conn, Message) error
    OnMessage(*Conn, *Buffer, int64)
}
