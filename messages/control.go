package messages

//go:generate msgp

import (
	"fmt"
	"time"

	"github.com/tinylib/msgp/msgp"
)

//messages sent directly over the control conn

// MessageType is an encoding for the underlying payload
type MessageType int

// definitions of message types
const (
	MsgUnsupported MessageType = iota
	MsgAuthControl
	MsgAuthTunnel
	MsgOpenTunnel
	MsgPing
	MsgPong
	MsgShutdown
	MsgRelease

	// insert new messagess above me
	msgEnd // for automated test generation
)

func typeToMessage(mt MessageType) Message {
	switch mt {
	case MsgAuthControl:
		return &AuthControl{}
	case MsgAuthTunnel:
		return &AuthTunnel{}
	case MsgOpenTunnel:
		return &OpenTunnel{}
	case MsgPing:
		return &Ping{}
	case MsgPong:
		return &Pong{}
	case MsgShutdown:
		return &Shutdown{}
	case MsgRelease:
		return &Release{}
	default:
		return nil
	}
}

func messageToType(m Message) MessageType {
	switch m.(type) {
	case *AuthControl:
		return MsgAuthControl
	case *AuthTunnel:
		return MsgAuthTunnel
	case *OpenTunnel:
		return MsgOpenTunnel
	case *Ping:
		return MsgPing
	case *Pong:
		return MsgPong
	case *Shutdown:
		return MsgShutdown
	case *Release:
		return MsgRelease
	default:
		return MsgUnsupported
	}
}

// Message is a generic interface for all the messages
type Message interface {
	msgp.Marshaler
	msgp.Unmarshaler
	msgp.Encodable
	msgp.Decodable
	msgp.Sizer
}

// Envelope is a wrapper struct used to encode message types as they are serialized to JSON
type Envelope struct {
	Type    MessageType `msg:"type"`
	Payload msgp.Raw    `msg:"payload"`
}

// AuthControl is sent by the client to create and authenticate a new session
type AuthControl struct {
	Token string `msg:"token"`
}

// AuthTunnel is sent by the client to create and authenticate a tunnel connection
type AuthTunnel struct {
	ClientID string `msg:"client_id"`
	Token    string `msg:"token"`
}

// OpenTunnel is sent by server to the client to request a new Tunnel connection
type OpenTunnel struct {
	ClientID string `msg:"client_id"`
}

// Shutdown is sent either by server or client to indicate that the session
// should be torn down
type Shutdown struct {
	Error string `msg:"error"`
}

// Ping is sent to request a Pong response and check the liveness of the connection
type Ping struct{}

// Pong is a response ot the Ping message
type Pong struct{}

// Release contains basic VCS (e.g. git) information about the running version
// of client server
type Release struct {
	ID                     string    `msg:"id" redis:"id,omitempty"`
	Branch                 string    `msg:"branch" redis:"branch,omitempty"`
	Description            string    `msg:"description" redis:"description,omitempty"`
	VCSType                string    `msg:"vcs_type" redis:"vcs_type,omitempty"`
	VCSRevision            string    `msg:"vcs_revision" redis:"vcs_revision,omitempty"`
	VCSRevisionMessage     string    `msg:"vcs_revision_message" redis:"vcs_revision_message,omitempty"`
	VCSRevisionTime        time.Time `msg:"vcs_revision_time" redis:"vcs_revision_time,omitempty"`
	VCSRevisionAuthorName  string    `msg:"vcs_revision_author_name" redis:"vcs_revision_author_name,omitempty"`
	VCSRevisionAuthorEmail string    `msg:"vcs_revision_author_email" redis:"vcs_revision_author_email,omitempty"`
}

func unpack(buffer []byte, msgIn Message) (msg Message, err error) {
	var env Envelope
	if _, err = env.UnmarshalMsg(buffer); err != nil {
		return
	}

	if msgIn == nil {
		msg = typeToMessage(env.Type)

		if msg == nil {
			err = fmt.Errorf("Unsupported message type %d", env.Type)
			return
		}
	} else {
		msg = msgIn
	}

	_, err = msg.UnmarshalMsg(env.Payload)
	return
}

// Unpack deserializes byte array into a message
func Unpack(buffer []byte) (msg Message, err error) {
	return unpack(buffer, nil)
}

// Pack serializes a message into a byte array
func Pack(payload Message) ([]byte, error) {
	mType := messageToType(payload)
	if mType == MsgUnsupported {
		return nil, fmt.Errorf("Unsupported message type")
	}

	raw, err := payload.MarshalMsg(nil)
	if err != nil {
		return nil, err
	}

	return (&Envelope{
		Type:    mType,
		Payload: raw,
	}).MarshalMsg(nil)
}
