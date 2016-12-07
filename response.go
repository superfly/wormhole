package wormhole

import (
	"errors"
	"io"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"
)

// Response ...
type Response struct {
	Ok     bool
	Errors []string
}

func AwaitResponse(stream io.ReadWriteCloser) (resp *Response, err error) {
	buf := make([]byte, 1024)
	nr, err := stream.Read(buf)
	if err != nil {
		return nil, errors.New("error reading from stream: " + err.Error())
	}
	err = msgpack.Unmarshal(buf[:nr], &resp)
	if err != nil {
		return nil, errors.New("unparsable response")
	}

	return resp, nil
}
