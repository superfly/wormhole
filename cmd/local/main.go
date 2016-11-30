package main // import "github.com/superfly/wormhole/cmd/local"

import (
	"errors"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/jpillora/backoff"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"

	log "github.com/Sirupsen/logrus"

	"github.com/superfly/wormhole"
)

const (
	noDelay      = 0
	interval     = 30
	resend       = 2
	noCongestion = 1
	maxBuffer    = 4194304
)

var (
	localEndpoint  = os.Getenv("LOCAL_ENDPOINT")
	remoteEndpoint = os.Getenv("REMOTE_ENDPOINT")
	smuxConfig     *smux.Config
	controlStream  *smux.Stream
)

const (
	clientID = "wormhole v0.0.1"
)

func init() {
	smuxConfig = smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = maxBuffer
	smuxConfig.KeepAliveInterval = 5 * time.Second
	smuxConfig.KeepAliveTimeout = 5 * time.Second
	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)
	if remoteEndpoint == "" {
		remoteEndpoint = ":10000"
	}
}

func main() {
	b := &backoff.Backoff{
		Max: 2 * time.Minute,
	}
	for {
		kcpconn, kcpconnErr := kcp.DialWithOptions(remoteEndpoint, nil, 10, 3)
		if kcpconnErr != nil {
			log.Errorln("Could not connect tunnel:", kcpconnErr)
			d := b.Duration()
			time.Sleep(d)
			continue
		}
		b.Reset()

		log.Println("Connected as:", kcpconn.LocalAddr().String())

		handleKCPConnection(kcpconn)
	}
}

func handleKCPConnection(kcpconn *kcp.UDPSession) {
	defer kcpconn.Close()

	setConnOptions(kcpconn)

	mux, err := smux.Client(kcpconn, smuxConfig)
	if err != nil {
		log.Fatalln(err)
	}
	defer mux.Close()
	handleProgramTermination(mux)

	stream, err := connect(mux)
	if err != nil {
		log.Errorln("Could not connect:", err)
		return
	}
	controlStream = stream

	err = authenticate(stream)
	if err != nil {
		log.Errorln("Could not authenticate:", err)
		return
	}

	log.Println("Authenticated.")

	go wormhole.InitPong(controlStream)

	for {
		stream, err := mux.AcceptStream()
		if err != nil { // Probably broken pipe...
			log.Errorln("Error accepting stream:", err)
			return
		}

		if err := handleStream(stream); err != nil {
			log.Errorln(err)
			continue
		}
	}
}

func connect(mux *smux.Session) (*smux.Stream, error) {
	stream, err := mux.OpenStream()
	if err != nil {
		return nil, errors.New("could not open initial stream: " + err.Error())
	}
	return stream, err
}

func handleProgramTermination(mux *smux.Session) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func(c <-chan os.Signal) {
		for _ = range c {
			log.Println("Cleaning up local agent.")
			mux.Close()
			os.Exit(1)
		}
	}(c)
}

func authenticate(stream *smux.Stream) error {
	hostname, err := os.Hostname()
	if err != nil {
		log.Debugln("Could not get hostname:", err)
		hostname = "unknown"
	}
	am := wormhole.AuthMessage{
		Token:  os.Getenv("FLY_TOKEN"),
		Name:   hostname,
		Client: clientID,
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
	resp, err := waitForResponse(stream)
	if err != nil {
		return errors.New("error waiting for authentication response: " + err.Error())
	}

	log.Printf("%+v", resp)

	if !resp.Ok {
		return errors.New("authentication failed")
	}

	return nil
}

func waitForResponse(stream *smux.Stream) (*wormhole.Response, error) {
	var resp wormhole.Response

	buf := make([]byte, 1024)
	nr, err := stream.Read(buf)
	if err != nil {
		return nil, errors.New("error reading from stream: " + err.Error())
	}
	err = msgpack.Unmarshal(buf[:nr], &resp)
	if err != nil {
		return nil, errors.New("unparsable auth message")
	}

	return &resp, nil
}

func handleStream(stream *smux.Stream) error {
	log.Debugln("Accepted stream")

	localConn, err := net.DialTimeout("tcp", localEndpoint, 5*time.Second)
	if err != nil {
		localConn.Close()
		return err
	}

	log.Debugln("dialed local connection")

	if err := localConn.(*net.TCPConn).SetReadBuffer(maxBuffer); err != nil {
		log.Errorln("TCP SetReadBuffer:", err)
	}
	if err := localConn.(*net.TCPConn).SetWriteBuffer(maxBuffer); err != nil {
		log.Errorln("TCP SetWriteBuffer:", err)
	}

	log.Debugln("local connection settings has been set...")

	go handleClient(localConn, stream)

	return nil
}

func setConnOptions(kcpconn *kcp.UDPSession) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(noDelay, interval, resend, noCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(1024, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(5)

	if err := kcpconn.SetDSCP(0); err != nil {
		log.Errorln("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(smuxConfig.MaxReceiveBuffer); err != nil {
		log.Errorln("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(smuxConfig.MaxReceiveBuffer); err != nil {
		log.Errorln("SetWriteBuffer:", err)
	}
}

func handleClient(c1, c2 io.ReadWriteCloser) {
	log.Debugln("c2 opened")
	defer log.Debugln("c2 closed")
	defer c1.Close()
	defer c2.Close()

	// start tunnel
	c1die := make(chan struct{})
	go func() { io.Copy(c1, c2); close(c1die) }()

	c2die := make(chan struct{})
	go func() { io.Copy(c2, c1); close(c2die) }()

	// wait for tunnel termination
	select {
	case <-c1die:
	case <-c2die:
	}
}
