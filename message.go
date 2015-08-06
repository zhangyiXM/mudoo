package mudoo

// The different message types that are available.
const (
    // MessagePlain is interpreted just as a string.
    MessagePlain = iota

    // MessageJSON is interpreted as a JSON encoded string.
    MessageJSON

    // MessageHeartbeat is interpreted as a heartbeat.
    MessageHeartbeat

    // MessageHandshake is interpreted as a handshake.
    MessageHandshake

    // MessageDisconnect is interpreted as a forced disconnection.
    MessageDisconnect
)

// Heartbeat is a server-invoked keep-alive strategy, where
// the server sends an integer to the client and the client
// must respond with the same value during some short period.
type heartbeat int

// Disconnect is a message that indicates a forced disconnection.
type disconnect int

// Handshake is the first message that is going to be sent to the
// client when it first connects. It is made of the server-generated
// session id.
type handshake string

// Message wraps heartbeat, messageType and data methods.
//
// Heartbeat returns the heartbeat value encapsulated in the message and
// an true or if the message does not encapsulate a heartbeat a false is
// returned.
// MessageType returns messagePlain, messageHeartBeat or messageJSON.
// Data returns the raw (full) message received.
type Message interface {
    heartbeat() (heartbeat, bool)

    Annotations() map[string]string
    Annotation(string) (string, bool)
    Data() string
    Bytes() []byte
    Type() uint8
    JSON() ([]byte, bool)
}
