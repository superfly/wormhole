package wormhole

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	git "gopkg.in/src-d/go-git.v4"
	msgpack "gopkg.in/vmihailenco/msgpack.v2"

	"github.com/jpillora/backoff"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"

	log "github.com/Sirupsen/logrus"
)

var (
	localEndpoint  = os.Getenv("LOCAL_ENDPOINT")
	remoteEndpoint = os.Getenv("REMOTE_ENDPOINT")
	controlStream  *smux.Stream
	cmd            *exec.Cmd
	flyToken       = os.Getenv("FLY_TOKEN")

	releaseIDVar   = os.Getenv("FLY_RELEASE_ID_VAR")
	releaseDescVar = os.Getenv("FLY_RELEASE_DESC_VAR")
	release        = &Release{}
)

func ensureLocalEnvironment() {
	if flyToken == "" {
		log.Fatalln("FLY_TOKEN is required, please set this environment variable.")
	}

	if releaseIDVar == "" {
		releaseIDVar = "FLY_RELEASE_ID"
	}

	if releaseDescVar == "" {
		releaseDescVar = "FLY_RELEASE_DESC"
	}

	smuxConfig = smux.DefaultConfig()
	smuxConfig.MaxReceiveBuffer = MaxBuffer
	smuxConfig.KeepAliveInterval = KeepAlive * time.Second
	// smuxConfig.KeepAliveTimeout = Interval * time.Second
	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)
	if remoteEndpoint == "" {
		remoteEndpoint = ":10000"
	}
	if version == "" {
		version = "latest"
	}
	if passphrase == "" {
		passphrase = os.Getenv("PASSPHRASE")
		if passphrase == "" {
			log.Fatalln("PASSPHRASE needs to be set")
		}
	}
	computeRelease()
}

func computeRelease() {
	if releaseID := os.Getenv(releaseIDVar); releaseID != "" {
		release.ID = releaseID
	}

	if releaseDesc := os.Getenv(releaseDescVar); releaseDesc != "" {
		release.Description = releaseDesc
	}

	if _, err := os.Stat(".git"); !os.IsNotExist(err) {
		release.VCSType = "git"
		repo, err := git.NewFilesystemRepository(".git")
		if err != nil {
			log.Warnln("Could not open repository:", err)
			return
		}
		ref, err := repo.Head()
		if err != nil {
			log.Warnln("Could not get repo head:", err)
			return
		}

		oid := ref.Hash()
		release.VCSRevision = oid.String()
		tip, err := repo.Commit(oid)
		if err != nil {
			log.Warnln("Could not get current commit:", err)
			return
		}
		author := tip.Author
		release.VCSRevisionAuthorEmail = author.Email
		release.VCSRevisionAuthorName = author.Name
		release.VCSRevisionTime = author.When
		release.VCSRevisionMessage = tip.Message
	}
	if release.ID == "" && release.VCSRevision != "" {
		release.ID = release.VCSRevision
	}
	if release.Description == "" && release.VCSRevisionMessage != "" {
		release.Description = release.VCSRevisionMessage
	}
	log.Println("current release:", release)
}

func runProgram(program string) (localPort string, err error) {
	cs := []string{"/bin/sh", "-c", program}
	cmd = exec.Command(cs[0], cs[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	localPort = os.Getenv("PORT")
	if localPort == "" {
		localPort = "5000"
		cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", 5000))
	} else {
		cmd.Env = os.Environ()
	}
	log.Println("Starting program:", program)
	err = cmd.Start()
	if err != nil {
		log.Println("Failed to start program:", err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				os.Exit(status.ExitStatus())
			}
		}
		return
	}
	go func(cmd *exec.Cmd) {
		err = cmd.Wait()
		if err != nil {
			log.Errorln("Program error:", err)
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					log.Printf("Exit Status: %d", status.ExitStatus())
					os.Exit(status.ExitStatus())
				}
			}
		}
		log.Println("Terminating program", program)
	}(cmd)
	return
}

// StartLocal ...
func StartLocal() {
	ensureLocalEnvironment()
	args := os.Args[1:]
	if len(args) > 0 {
		localPort, err := runProgram(strings.Join(args, " "))
		if err != nil {
			log.Errorln("Error running program:", err)
			return
		}
		localEndpoint = "127.0.0.1:" + localPort
	}

	b := &backoff.Backoff{
		Max: 2 * time.Minute,
	}
	go DebugSNMP()
	for {
		mux, err := initializeConnection()
		if err != nil {
			log.Errorln("Could not make connection:", err)
			d := b.Duration()
			time.Sleep(d)
			continue
		}
		defer controlStream.Close()
		b.Reset()
		handleMux(mux)
	}
}

func initializeConnection() (*smux.Session, error) {
	block, _ := kcp.NewAESBlockCrypt([]byte(passphrase)[:32])
	kcpconn, kcpconnErr := kcp.DialWithOptions(remoteEndpoint, block, 10, 3)
	if kcpconnErr != nil {
		return nil, kcpconnErr
	}
	log.Println("Connected as:", kcpconn.LocalAddr().String())

	setConnOptions(kcpconn)

	mux, err := smux.Client(kcpconn, smuxConfig)
	if err != nil {
		log.Errorln("Error creating multiplexed session:", err)
		return nil, err
	}
	handleOsSignal(mux)

	controlStream, err = handshakeConnection(mux)
	if err != nil {
		return nil, err
	}
	return mux, nil
}

func handshakeConnection(mux *smux.Session) (*smux.Stream, error) {
	stream, err := connect(mux)
	if err != nil {
		log.Errorln("Could not connect:", err)
		return nil, err
	}

	err = authenticate(stream)
	if err != nil {
		log.Errorln("Could not authenticate:", err)
		defer stream.Close()
		return nil, err
	}

	log.Println("Authenticated.")
	return stream, nil

}

func stayAlive() {
	err := InitPong(controlStream)
	if err != nil {
		log.Errorln("PONG error:", err)
	}
	log.Println("Pong ended.")
}

func handleMux(mux *smux.Session) error {
	defer mux.Close()
	go stayAlive()

	for {
		stream, err := mux.AcceptStream()
		if err != nil { // Probably broken pipe...
			log.Errorln("Error accepting stream:", err)
			return err
		}

		go handleStream(stream)
	}
}

func connect(mux *smux.Session) (*smux.Stream, error) {
	stream, err := mux.OpenStream()
	if err != nil {
		return nil, errors.New("could not open initial stream: " + err.Error())
	}
	return stream, err
}

func signalProcess(sig os.Signal) (exited bool, exitStatus int, err error) {
	exited = false
	err = cmd.Process.Signal(sig)
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			exited = true
			status, _ := exiterr.Sys().(syscall.WaitStatus)
			exitStatus = status.ExitStatus()
		}
	}
	return
}

func handleOsSignal(mux *smux.Session) {
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan)
	go func(signalChan <-chan os.Signal) {
		for sig := range signalChan {
			var exitStatus int
			var exited bool
			if cmd != nil {
				exited, exitStatus, _ = signalProcess(sig)
			} else {
				exitStatus = 0
			}
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Cleaning up local agent.")
				mux.Close()
				os.Exit(int(exitStatus))
			default:
				if cmd != nil && exited {
					os.Exit(int(exitStatus))
				}
			}
		}
	}(signalChan)
}

func authenticate(stream *smux.Stream) error {
	hostname, err := os.Hostname()
	if err != nil {
		log.Debugln("Could not get hostname:", err)
		hostname = "unknown"
	}
	am := AuthMessage{
		Token:   flyToken,
		Name:    hostname,
		Client:  "wormhole " + version,
		Release: release,
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
	resp, err := AwaitResponse(stream)
	if err != nil {
		return errors.New("error waiting for authentication response: " + err.Error())
	}

	log.Printf("%+v", resp)

	if !resp.Ok {
		return errors.New("authentication failed")
	}

	return nil
}

func handleStream(stream *smux.Stream) (err error) {
	log.Debugln("Accepted stream")

	localConn, err := net.DialTimeout("tcp", localEndpoint, 5*time.Second)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debugln("dialed local connection")

	if err = localConn.(*net.TCPConn).SetReadBuffer(MaxBuffer); err != nil {
		log.Errorln("TCP SetReadBuffer error:", err)
	}
	if err = localConn.(*net.TCPConn).SetWriteBuffer(MaxBuffer); err != nil {
		log.Errorln("TCP SetWriteBuffer error:", err)
	}

	log.Debugln("local connection settings has been set...")

	err = CopyCloseIO(localConn, stream)
	return err
}

func setConnOptions(kcpconn *kcp.UDPSession) {
	kcpconn.SetStreamMode(true)
	kcpconn.SetNoDelay(NoDelay, Interval, Resend, NoCongestion)
	kcpconn.SetMtu(1350)
	kcpconn.SetWindowSize(128, 1024)
	kcpconn.SetACKNoDelay(true)
	kcpconn.SetKeepAlive(KeepAlive)

	if err := kcpconn.SetDSCP(DSCP); err != nil {
		log.Errorln("SetDSCP:", err)
	}
	if err := kcpconn.SetReadBuffer(smuxConfig.MaxReceiveBuffer); err != nil {
		log.Errorln("SetReadBuffer:", err)
	}
	if err := kcpconn.SetWriteBuffer(smuxConfig.MaxReceiveBuffer); err != nil {
		log.Errorln("SetWriteBuffer:", err)
	}
}
