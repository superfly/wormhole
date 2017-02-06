package messages

import (
	"encoding/json"
	"fmt"
	"reflect"
)

//messages sent directly over the wire

// TypeMap stores a list of messages types and provides a way to deserialize messages
var TypeMap map[string]reflect.Type

func init() {
	TypeMap = make(map[string]reflect.Type)

	t := func(obj interface{}) reflect.Type { return reflect.TypeOf(obj).Elem() }
	TypeMap["AuthControl"] = t((*AuthControl)(nil))
	TypeMap["AuthTunnel"] = t((*AuthTunnel)(nil))
	TypeMap["OpenTunnel"] = t((*OpenTunnel)(nil))
	TypeMap["Ping"] = t((*Ping)(nil))
	TypeMap["Pong"] = t((*Pong)(nil))
	TypeMap["Shutdown"] = t((*Shutdown)(nil))
}

// Message is a generic interface for all the messages
type Message interface{}

// Envelope is a wrapper struct used to encode message types as they are serialized to JSON
type Envelope struct {
	Type    string
	Payload json.RawMessage
}

// AuthControl is sent by the client to create and authenticate a new session
type AuthControl struct {
	Token string
}

// AuthTunnel is sent by the client to create and authenticate a tunnel connection
type AuthTunnel struct {
	ClientID string
	Token    string
}

// OpenTunnel is sent by server to the client to request a new Tunnel connection
type OpenTunnel struct {
	ClientID string
}

// Shutdown is sent either by server or client to indicate that the session
// should be torn down
type Shutdown struct {
	Error string
}

// Ping is sent to request a Pong response and check the liveness of the connection
type Ping struct{}

// Pong is a response ot the Ping message
type Pong struct{}

func unpack(buffer []byte, msgIn Message) (msg Message, err error) {
	var env Envelope
	if err = json.Unmarshal(buffer, &env); err != nil {
		return
	}

	if msgIn == nil {
		t, ok := TypeMap[env.Type]

		if !ok {
			err = fmt.Errorf("Unsupported message type %s", env.Type)
			return
		}

		// guess type
		msg = reflect.New(t).Interface().(Message)
	} else {
		msg = msgIn
	}

	err = json.Unmarshal(env.Payload, &msg)
	return
}

// Unpack deserializes byte array into a message
func Unpack(buffer []byte) (msg Message, err error) {
	return unpack(buffer, nil)
}

// Pack serializes a message into a byte array
func Pack(payload interface{}) ([]byte, error) {
	return json.Marshal(struct {
		Type    string
		Payload interface{}
	}{
		Type:    reflect.TypeOf(payload).Elem().Name(),
		Payload: payload,
	})
}
