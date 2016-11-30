package wormhole

import (
	"errors"
	"time"

	"github.com/xtaci/smux"
)

const (
	ping = "ping"
	pong = "pong"
)

// InitPing ...
func InitPing(stream *smux.Stream) (err error) {
	for {
		stream.Write([]byte(ping))
		stream.SetDeadline(time.Now().Add(time.Second))
		readbuf := make([]byte, 4)
		_, err = stream.Read(readbuf)
		if err != nil {
			break
		}
		if string(readbuf) != pong {
			err = errors.New("Unexpected response to ping: " + string(readbuf))
			break
		}
		time.Sleep(5 * time.Second)
	}
	return err
}

// InitPong ...
func InitPong(stream *smux.Stream) (err error) {
	for {
		readbuf := make([]byte, 4)
		_, err = stream.Read(readbuf)
		if err != nil {
			break
		}
		if string(readbuf) != ping {
			err = errors.New("Unexpected ping request: " + string(readbuf))
			break
		}
		stream.Write([]byte(pong))
	}
	return err
}
