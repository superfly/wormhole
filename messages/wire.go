package messages

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

//messages sent directly over the wire

var TypeMap map[string]reflect.Type

func init() {
	TypeMap = make(map[string]reflect.Type)

	t := func(obj interface{}) reflect.Type { return reflect.TypeOf(obj).Elem() }
	TypeMap["AuthControl"] = t((*AuthControl)(nil))
	TypeMap["AuthTunnel"] = t((*AuthTunnel)(nil))
	TypeMap["OpenTunnel"] = t((*OpenTunnel)(nil))
	TypeMap["Shutdown"] = t((*Shutdown)(nil))
}

type Message interface{}

type Envelope struct {
	Type    string
	Payload json.RawMessage
}

type AuthControl struct {
	Token string
}

type AuthTunnel struct {
	ClientID string
	Token    string
}

type OpenTunnel struct {
	ClientID string
}

type Shutdown struct {
	Error string
}

func unpack(buffer []byte, msgIn Message) (msg Message, err error) {
	var env Envelope
	if err = json.Unmarshal(buffer, &env); err != nil {
		return
	}

	if msgIn == nil {
		t, ok := TypeMap[env.Type]

		if !ok {
			err = errors.New(fmt.Sprintf("Unsupported message type %s", env.Type))
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

func Unpack(buffer []byte) (msg Message, err error) {
	return unpack(buffer, nil)
}

func Pack(payload interface{}) ([]byte, error) {
	return json.Marshal(struct {
		Type    string
		Payload interface{}
	}{
		Type:    reflect.TypeOf(payload).Elem().Name(),
		Payload: payload,
	})
}
