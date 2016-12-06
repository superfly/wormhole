package wormhole

import (
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
