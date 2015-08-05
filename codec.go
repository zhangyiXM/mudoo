package mudoo

import (
    "bytes"
    "io"
    "os"
)

var (
    ErrMalformedPayload = os.NewError("malformed payload")
)

// A Codec wraps Encode and Decode methods.
//
// Encode takes an interface{}, encodes it and writes it to the given payload
// can't be decoded, an ErrmalformedPayload error will be returned.
type Codec interface {
    NewEncoder() Encoder
    NewDecoder(*bytes.Buffer) Decoder
}

type Decoder interface {
    Decode() ([]Message, os.Error)
    Reset()
}

type Encoder interface {
    Encode(io.Writer, interface{}) os.Error
}
