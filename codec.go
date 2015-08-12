package mudoo

// A Codec wraps Encode and Decode methods.
type Codec interface {
    OnMessage(*Conn, *Buffer, int64)
}
