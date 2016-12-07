package wormhole

import (
	"io"
	"log"
	"time"

	kcp "github.com/xtaci/kcp-go"
)

// Constants, useful to both server and client
const (
	NoDelay      = 0
	Interval     = 30
	Resend       = 2
	NoCongestion = 1
	MaxBuffer    = 4194304
	KeepAlive    = 10

	// KCP
	KCPShards = 10
	KCPParity = 3
	DSCP      = 0
)

// OutputSNMP ...
func OutputSNMP() {
	log.Printf("KCP SNMP:%+v", kcp.DefaultSnmp.Copy())
}

// DebugSNMP ...
func DebugSNMP() {
	for _ = range time.Tick(120 * time.Second) {
		OutputSNMP()
	}
}

// CopyCloseIO ...
func CopyCloseIO(c1, c2 io.ReadWriteCloser) (err error) {
	defer c1.Close()
	defer c2.Close()

	// start tunnel
	c1die := make(chan struct{})
	go func() {
		_, err = io.Copy(c1, c2)
		close(c1die)
	}()

	c2die := make(chan struct{})
	go func() {
		_, err = io.Copy(c2, c1)
		close(c2die)
	}()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
	return err
}
