package handler

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/superfly/wormhole/session"
	"golang.org/x/crypto/ssh"
)

const (
	sshRemoteForwardRequest      = "tcpip-forward"
	sshForwardedTCPReturnRequest = "forwarded-tcpip"
)

type SshHandler struct {
	ln     net.Listener
	config *ssh.ServerConfig
	Port   string
}

func (s *SshHandler) InitializeConnection() error {
	config, err := makeConfig()
	if err != nil {
		return fmt.Errorf("Failed to build ssh server config: %s", err)
	}
	s.config = config

	listener, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		return fmt.Errorf("Failed to listen on %s (%s)", s.Port, err)
	}
	s.ln = listener

	return nil
}

func (s *SshHandler) ListenAndServe() {
	for {
		tcpConn, err := s.ln.Accept()
		if err != nil {
			log.Errorf("Failed to accept incoming connection (%s)", err)
			break
		}
		defer tcpConn.Close()

		// Before use, a handshake must be performed on the incoming net.Conn.
		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, s.config)
		if err != nil {
			log.Printf("Failed to handshake (%s)", err)
			break
		}

		log.Printf("New SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())
		// Discard all global out-of-band Requests
		go handleRequests(reqs, sshConn)
		// Accept all channels
		go handleChannels(chans)
	}
}

func (s *SshHandler) Close() error {
	return nil
}

func makeConfig() (*ssh.ServerConfig, error) {
	config := &ssh.ServerConfig{
		PasswordCallback: authFromToken,
	}

	if private, err := getPrivateKey(); err == nil {
		config.AddHostKey(private)
	} else {
		return nil, err
	}

	return config, nil
}

func authFromToken(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	/*
		passString := string(pass)
			backendID, err := BackendIDFromToken(passString)

			if err != nil {
				return nil, err
			}
			if backendID == "" {
				return nil, errors.New("token rejected " + passString)
			}

				session := Session{
					ID:            hex.EncodeToString(c.SessionID()),
					User:          c.User(),
					RemoteAddr:    c.RemoteAddr().String(),
					ClientVersion: string(c.ClientVersion()),
					NodeID:        nodeID,
					BackendID:     backendID,
				}
				sessionsByID[session.ID] = session

				go session.RegisterConnection(time.Now())

				log.WithFields(log.Fields{
					"session": session,
				}).Debug("Session stored in memory.")
	*/
	return nil, nil

}

func getPrivateKey() (ssh.Signer, error) {
	privateBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		return nil, errors.New("Failed to load private key (./id_rsa)")
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, errors.New("Failed to parse private key")
	}

	return private, nil
}

func handleRequests(reqs <-chan *ssh.Request, sshConn ssh.Conn) {
	//session := sessionsByID[hex.EncodeToString(sshConn.SessionID())]
	// Service the incoming Channel channel in go routine
	for req := range reqs {
		switch req.Type {
		case sshRemoteForwardRequest:
			go func() {
				//handleRemoteForward(req, sshConn, session)
				//go session.RegisterDisconnection(time.Now())
			}()
		case "keepalive":
			//go handleKeepalive(req, sshConn, session)
		}
	}
}

func handleKeepalive(req *ssh.Request, sshConn ssh.Conn, session session.Session) {
	if req.WantReply {
		req.Reply(true, nil)
	}
	//go session.RegisterKeepalive(time.Now())
}

func handleChannels(chans <-chan ssh.NewChannel) {
	for _ = range chans {
		// nothing for now.
	}
}

type tcpipForward struct {
	Host string
	Port uint32
}

type directForward struct {
	Host1 string
	Port1 uint32
	Host2 string
	Port2 uint32
}

func handleRemoteForward(req *ssh.Request, sshConn ssh.Conn, session session.Session) {

	t := tcpipForward{}
	ssh.Unmarshal(req.Payload, &t)

	reply := (t.Port == 0) && req.WantReply
	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	ln, err := net.Listen("tcp4", addr) //tie to the client connection

	if err != nil {
		log.Errorln("Unable to listen on address:", addr)
		return
	}
	log.Infoln("Listening on address: ", ln.Addr().String())

	quit := make(chan bool)
	if reply { // Client sent port 0. let them know which port is actually being used
		_, port, _ := getHostPortFromAddr(ln.Addr())
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(port))
		t.Port = uint32(port)
		req.Reply(true, b)
	} else {
		req.Reply(true, nil)
	}

	/*
		tunnelIP, _, _ := net.SplitHostPort(sshConn.LocalAddr().String())
		_, tunnelPort, _ := net.SplitHostPort(ln.Addr().String())

			go session.UpdateAttribute("tunnel_address", tunnelIP+":"+tunnelPort)
					endpoint := &Endpoint{
						BackendID: session.BackendID,
						SessionID: session.ID,
						IP:        tunnelIP,
						Port:      tunnelPort,
						Name:      session.User,
					}

				if err = endpoint.Register(); err != nil {
					log.Errorln("Error registering endpoint:", err)
				}
	*/

	go func(ln net.Listener) { // Handle incoming connections on this new listener
		for {
			select {
			case <-quit:
				return
			default:
				conn, connErr := ln.Accept()
				if connErr != nil { // Unable to accept new connection - listener likely closed
					continue
				}
				go func(conn net.Conn) {
					p := directForward{}

					var portnum int
					p.Host1 = t.Host
					p.Port1 = t.Port
					p.Host2, portnum, err = getHostPortFromAddr(conn.RemoteAddr())
					if err != nil {
						return
					}

					p.Port2 = uint32(portnum)
					/*
						          ch, reqs, sshErr := sshConn.OpenChannel(sshForwardedTCPReturnRequest, ssh.Marshal(p))
											if sshErr != nil {
												log.WithFields(log.Fields{
													"err": err.Error(),
												}).Error("Open forwarded Channel error:")
												return
											}
											go ssh.DiscardRequests(reqs)

												go copyReadWriters(conn, ch, func() {
													ch.Close()
													conn.Close()
												})
					*/

				}(conn)
			}

		}

	}(ln)

	sshConn.Wait()
	ln.Close()

	//go endpoint.Remove()

	quit <- true
}

func getHostPortFromAddr(addr net.Addr) (host string, port int, err error) {
	host, portString, err := net.SplitHostPort(addr.String())
	if err != nil {
		return
	}
	port, err = strconv.Atoi(portString)
	return
}
