package main

import "github.com/xtaci/smux"

// ControlStream ...
type ControlStream struct {
	SessionID string
	Stream    *smux.Stream
}
