package mudoo

type Message interface {
    heartbeat() (heartbeat, bool)

    Annotations() map[string]string
    Annotation(string) (string, bool)
    Data() string
    Bytes() []byte
    Type() uint8
    JSON() ([]byte, bool)
}
