package handler

import (
	"errors"
	"net"
	"os"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/smux"

	"github.com/superfly/wormhole/messages"

	"github.com/superfly/wormhole/utils"
	kcp "github.com/xtaci/kcp-go"
)

const (
	noDelay      = 0
	interval     = 30
	resend       = 2
	noCongestion = 1
	maxBuffer    = 4194304
	keepAlive    = 10

	// KCP
	kcpShards = 10
	kcpParity = 3
	dscp      = 0

	secretLength = 32
)

type ConnectionHandler interface {
	InitializeConnection() error
	Close() error
}

type SmuxHandler struct {
	Passphrase     string
	RemoteEndpoint string
	LocalEndpoint  string
	smux           *smux.Session
	controlStream  *smux.Stream
	FlyToken       string
	Release        *messages.Release
	Version        string
	Config         *smux.Config
}

// InitializeConnection ...
func (s *SmuxHandler) InitializeConnection() error {
	block, _ := kcp.NewAESBlockCrypt([]byte(s.Passphrase)[:secretLength])
	kcpconn, kcpconnErr := kcp.DialWithOptions(s.RemoteEndpoint, block, kcpShards, kcpParity)
	if kcpconnErr != nil {
		return kcpconnErr
	}
	log.Println("Connected as:", kcpconn.LocalAddr().String())

	setConnOptions(kcpconn, s.Config)

	mux, err := smux.EncryptedClient(kcpconn, s.Config)
	if err != nil {
		log.Errorln("Error creating multiplexed session:", err)
		return err
	}
	s.smux = mux

	controlStream, err := s.handshakeConnection(mux)
	if err != nil {
		return err
	}
	s.controlStream = controlStream
	return nil
}

func (s *SmuxHandler) Close() error {
	return s.smux.Close()
}

func (s *SmuxHandler) ListenAndServe() error {
	defer s.smux.Close()
	go s.stayAlive()

	for {
		stream, err := s.smux.AcceptStream()
		if err != nil { // Probably broken pipe...
			log.Errorln("Error accepting stream:", err)
			return err
		}

		go handleStream(stream, s.LocalEndpoint)
	}
}

func (s *SmuxHandler) handshakeConnection(mux *smux.Session) (*smux.Stream, error) {
	stream, err := connect(mux)
	if err != nil {
		log.Errorln("Could not connect:", err)
		return nil, err
	}

	err = s.authenticate(stream)
	if err != nil {
		log.Errorln("Could not authenticate:", err)
		defer stream.Close()
		return nil, err
	}

	log.Println("Authenticated.")
	return stream, nil

}

func (s *SmuxHandler) authenticate(stream *smux.Stream) error {
	hostname, err := os.Hostname()
	if err != nil {
		log.Debugln("Could not get hostname:", err)
		hostname = "unknown"
	}
	am := messages.AuthMessage{
		Token:   s.FlyToken,
		Name:    hostname,
		Client:  "wormhole " + s.Version,
		Release: s.Release,
	}
	buf, err := msgpack.Marshal(am)
	if err != nil {
		return errors.New("could not serialize auth message: " + err.Error())
	}
	log.Println("Authenticating...")
	_, err = stream.Write(buf)
	if err != nil {
		return errors.New("could not write auth message: " + err.Error())
	}

	log.Debugln("Waiting for authentication answer...")
	resp, err := messages.AwaitResponse(stream)
	if err != nil {
		return errors.New("error waiting for authentication response: " + err.Error())
	}

	log.Printf("%+v", resp)

	if !resp.Ok {
		return errors.New("authentication failed")
	}

	return nil
}

func handleStream(stream *smux.Stream, localEndpoint string) (err error) {
	log.Debugln("Accepted stream")

	localConn, err := net.DialTimeout("tcp", localEndpoint, 5*time.Second)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debugln("dialed local connection")

	if err = localConn.(*net.TCPConn).SetReadBuffer(maxBuffer); err != nil {
		log.Errorln("TCP SetReadBuffer error:", err)
	}
	if err = localConn.(*net.TCPConn).SetWriteBuffer(maxBuffer); err != nil {
		log.Errorln("TCP SetWriteBuffer error:", err)
	}

	log.Debugln("local connection settings has been set...")

	err = utils.CopyCloseIO(localConn, stream)
	return err
}

func connect(mux *smux.Session) (*smux.Stream, error) {
	stream, err := mux.OpenStream()
	if err != nil {
		return nil, errors.New("could not open initial stream: " + err.Error())
	}
	return stream, err
}

func setConnOptions(kcpconn *kcp.UDPSession, config *smux.Config) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(noDelay, interval, resend, noCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(128, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(keepAlive)

	if err := kcpconn.SetDSCP(dscp); err != nil {
		log.Errorln("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(config.MaxReceiveBuffer); err != nil {
		log.Errorln("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(config.MaxReceiveBuffer); err != nil {
		log.Errorln("SetWriteBuffer:", err)
	}
}

func (s *SmuxHandler) stayAlive() {
	err := initPong(s.controlStream)
	if err != nil {
		log.Errorln("PONG error:", err)
	}
	log.Println("Pong ended.")
}

func initPong(stream *smux.Stream) (err error) {
	const (
		ping = "ping"
		pong = "pong"
	)
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
