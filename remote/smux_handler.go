package handler

import (
	log "github.com/Sirupsen/logrus"

	config "github.com/superfly/wormhole/shared"
	"github.com/superfly/wormhole/utils"
	kcp "github.com/xtaci/kcp-go"
)

type ConnectionHandler interface {
	InitializeConnection() error
	Close() error
}

type SmuxHandler struct {
	Passphrase string
	ListenPort string
	ln         *kcp.Listener
}

// InitializeConnection ...
func (s *SmuxHandler) InitializeConnection() error {
	block, _ := kcp.NewAESBlockCrypt([]byte(s.Passphrase)[:config.SecretLength])
	ln, err := kcp.ListenWithOptions(":"+s.ListenPort, block, config.KCPShards, config.KCPParity)
	if err != nil {
		log.Fatalln("KCP Server:", err)
		return err
	}
	s.ln = ln

	if err = ln.SetDSCP(config.DSCP); err != nil {
		log.Warnln("SetDSCP:", err)
		// not fatal
		return nil
	}
	if err = ln.SetReadBuffer(config.MaxBuffer); err != nil {
		log.Fatalln("SetReadBuffer:", err)
		return err
	}
	if err = ln.SetWriteBuffer(config.MaxBuffer); err != nil {
		log.Fatalln("SetWriteBuffer:", err)
		return err
	}

	log.Println("Listening on", ln.Addr().String())
	go utils.DebugSNMP()

	return nil
}

func (s *SmuxHandler) Close() {
	s.ln.Close()
}

func (s *SmuxHandler) ListenAndServe(fn func(*kcp.UDPSession)) {
	for {
		kcpconn, err := s.ln.AcceptKCP()
		if err != nil {
			log.Errorln("error accepting KCP:", err)
			break
		}
		go handleConn(kcpconn, fn)
		log.Println("Accepted connection from:", kcpconn.RemoteAddr())
	}
	log.Println("Stopping server KCP...")
}

func setRemoteConnOptions(kcpconn *kcp.UDPSession) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(128, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(config.KeepAlive)
}

func handleConn(kcpconn *kcp.UDPSession, fn func(*kcp.UDPSession)) {
	defer kcpconn.Close()
	setRemoteConnOptions(kcpconn)
	fn(kcpconn)
}
