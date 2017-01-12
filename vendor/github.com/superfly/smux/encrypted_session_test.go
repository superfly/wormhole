package smux

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/box"
)

var (
	testServerPubKey  *[32]byte
	testServerPrivKey *[32]byte
)

func init() {
	pubKey, privKey, err := box.GenerateKey(crand.Reader)
	testServerPrivKey = privKey
	testServerPubKey = pubKey
	if err != nil {
		panic(err)
	}
	go func() {
		log.Println(http.ListenAndServe("localhost:6061", nil))
	}()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ln, err := net.Listen("tcp", "127.0.0.1:19998")
	if err != nil {
		// handle error
		panic(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// handle error
			}
			go handleEncryptedConnection(conn)
		}
	}()
}

func newTestClient(conn net.Conn) (*Session, error) {
	config := DefaultConfig()
	config.ServerPublicKey = *testServerPubKey
	config.KeyHandshakeTimeout = 1 * time.Second
	return EncryptedClient(conn, config)
}

func newTestServer(conn net.Conn) (*Session, error) {
	config := DefaultConfig()
	config.ServerPrivateKey = *testServerPrivKey
	config.ServerPublicKey = *testServerPubKey
	config.KeyHandshakeTimeout = 1 * time.Second
	return EncryptedServer(conn, config)
}

func handleEncryptedConnection(conn net.Conn) error {
	session, err := newTestServer(conn)
	if err != nil {
		return err
	}
	for {
		if stream, err := session.AcceptStream(); err == nil {
			go func(s io.ReadWriteCloser) {
				buf := make([]byte, 65536)
				for {
					n, err := s.Read(buf)
					if err != nil {
						return
					}
					s.Write(buf[:n])
				}
			}(stream)
		} else {
			return err
		}
	}
}

func TestEncryptedEcho(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	const N = 100
	buf := make([]byte, 10)
	var sent string
	var received string
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		stream.Write([]byte(msg))
		sent += msg
		if n, err := stream.Read(buf); err != nil {
			t.Fatal(err)
		} else {
			received += string(buf[:n])
		}
	}
	if sent != received {
		t.Fatal("data mimatch")
	}
	session.Close()
}

func TestEncryptedSpeed(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(stream.LocalAddr(), stream.RemoteAddr())

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		buf := make([]byte, 1024*1024)
		nrecv := 0
		for {
			n, err := stream.Read(buf)
			if err != nil {
				t.Fatal(err)
				break
			} else {
				nrecv += n
				if nrecv == 4096*4096 {
					break
				}
			}
		}
		stream.Close()
		t.Log("time for 16MB rtt", time.Since(start))
		wg.Done()
	}()
	msg := make([]byte, 8192)
	for i := 0; i < 2048; i++ {
		stream.Write(msg)
	}
	wg.Wait()
	session.Close()
}

func TestEncryptedParallel(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}

	par := 1000
	messages := 100
	var wg sync.WaitGroup
	wg.Add(par)
	for i := 0; i < par; i++ {
		stream, err := session.OpenStream()
		if err != nil {
			t.Fatal(err)
		}
		go func(s *Stream) {
			buf := make([]byte, 20)
			for j := 0; j < messages; j++ {
				msg := fmt.Sprintf("hello%v", j)
				s.Write([]byte(msg))
				if _, err := s.Read(buf); err != nil {
					break
				}
			}
			s.Close()
			wg.Done()
		}(stream)
	}
	t.Log("created", session.NumStreams(), "streams")
	wg.Wait()
	session.Close()
}

func TestEncryptedParallelServerSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:39978")
	if err != nil {
		t.Fatal(err)
	}
	par := 1000
	messages := 100
	var sg, cg sync.WaitGroup
	sg.Add(par)
	cg.Add(par)

	type streamData struct {
		streamID int
		msg      string
	}
	sentCh := make(chan streamData)
	receivedCh := make(chan streamData)
	die := make(chan bool)
	done := make(chan bool)
	errCh := make(chan error)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
		}
		session, err := newTestServer(conn)
		if err != nil {
			errCh <- err
		}
		for i := 0; i < par; i++ {
			defer sg.Done()
			go func(i int) {
				select {
				case <-die:
					return
				default:
				}

				stream, err := session.OpenStream()
				if err != nil {
					errCh <- err
				}
				var sent string
				for j := 0; j < messages; j++ {
					select {
					case <-die:
						return
					default:
					}
					msg := fmt.Sprintf("hello%v-%v", i, j)
					if _, err := stream.Write([]byte(msg)); err != nil {
						errCh <- err
					}
					sent += msg
				}
				stream.Close()
				sentCh <- streamData{streamID: int(stream.ID()), msg: sent}
			}(i)
		}
	}()

	cli, err := net.Dial("tcp", "127.0.0.1:39978")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < par; i++ {
		go func(i int) {
			select {
			case <-die:
				return
			default:
			}
			stream, err := session.AcceptStream()
			if err != nil {
				errCh <- err
			}
			buf := make([]byte, 65536)
			var received string
			for {
				select {
				case <-die:
					return
				default:
				}
				n, err := stream.Read(buf)
				if err != nil && err == io.EOF {
					break
				} else if err != nil {
					errCh <- err
				}
				received += string(buf[:n])
			}
			receivedCh <- streamData{streamID: int(stream.ID()), msg: received}
			cg.Done()
		}(i)
	}

	go func() {
		sg.Wait()
		cg.Wait()
		close(done)
	}()

	received := make(map[int]string)
	sent := make(map[int]string)

	for {
		select {
		case s := <-sentCh:
			sent[s.streamID] = s.msg
		case r := <-receivedCh:
			received[r.streamID] = r.msg
		case err := <-errCh:
			close(die)
			t.Fatal(err)
		case <-done:
			for k := range received {
				if received[k] != sent[k] {
					t.Fatalf("Expected sent '%s' to equal received: '%s'\n", sent[k], received[k])
				}
			}
			return
		}
	}
}

func TestEncryptedCloseThenOpen(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
	if _, err := session.OpenStream(); err == nil {
		t.Fatal("opened after close")
	}
}

func TestEncryptedStreamDoubleClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := session.OpenStream()
	stream.Close()
	if err := stream.Close(); err == nil {
		t.Log("double close doesn't return error")
	}
	session.Close()
}

func TestEncryptedConcurrentClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	numStreams := 100
	streams := make([]*Stream, 0, numStreams)
	var wg sync.WaitGroup
	wg.Add(numStreams)
	for i := 0; i < 100; i++ {
		stream, err := session.OpenStream()
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, stream)
	}
	for _, s := range streams {
		stream := s
		go func() {
			stream.Close()
			wg.Done()
		}()
	}
	session.Close()
	wg.Wait()
}

func TestEncryptedTinyReadBuffer(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	const N = 100
	tinybuf := make([]byte, 6)
	var sent string
	var received string
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		sent += msg
		nsent, err := stream.Write([]byte(msg))
		if err != nil {
			t.Fatal("cannot write")
		}
		nrecv := 0
		for nrecv < nsent {
			if n, err := stream.Read(tinybuf); err == nil {
				nrecv += n
				received += string(tinybuf[:n])
			} else {
				t.Fatal("cannot read with tiny buffer")
			}
		}
	}

	if sent != received {
		t.Fatal("data mimatch")
	}
	session.Close()
}

func TestEncryptedIsClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
	if session.IsClosed() != true {
		t.Fatal("still open after close")
	}
}

func TestEncryptedKeepAliveTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:29998")
	if err != nil {
		// handle error
		panic(err)
	}
	go func() {
		ln.Accept()
	}()

	cli, err := net.Dial("tcp", "127.0.0.1:29998")
	if err != nil {
		t.Fatal(err)
	}

	config := DefaultConfig()
	config.ServerPublicKey = *testServerPubKey
	config.KeepAliveInterval = time.Second
	config.KeepAliveTimeout = 2 * time.Second
	session, err := Client(cli, config)
	if err != nil {
		t.Fatal(err)
	}
	<-time.After(3 * time.Second)
	if session.IsClosed() != true {
		t.Fatal("keepalive-timeout failed")
	}
}

func TestEncryptedServerEcho(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:39998")
	if err != nil {
		// handle error
		t.Fatal(err)
	}
	go func() {
		if conn, err := ln.Accept(); err == nil {
			session, err := newTestServer(conn)
			if err != nil {
				t.Fatal(err)
			}
			if stream, err := session.OpenStream(); err == nil {
				const N = 100
				buf := make([]byte, 10)
				for i := 0; i < N; i++ {
					msg := fmt.Sprintf("hello%v", i)
					stream.Write([]byte(msg))
					if n, err := stream.Read(buf); err != nil {
						t.Fatal(err)
					} else if string(buf[:n]) != msg {
						t.Fatal(err)
					}
				}
				stream.Close()
			} else {
				t.Fatal(err)
			}
		} else {
			t.Fatal(err)
		}
	}()

	cli, err := net.Dial("tcp", "127.0.0.1:39998")
	if err != nil {
		t.Fatal(err)
	}
	if session, err := newTestClient(cli); err == nil {
		if stream, err := session.AcceptStream(); err == nil {
			buf := make([]byte, 65536)
			for {
				n, err := stream.Read(buf)
				if err != nil {
					break
				}
				stream.Write(buf[:n])
			}
		} else {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
}

func TestEncryptedServerSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:39988")
	if err != nil {
		// handle error
		t.Fatal(err)
	}
	sentCh := make(chan string)
	go func() {
		if conn, err := ln.Accept(); err == nil {
			session, err := newTestServer(conn)
			if err != nil {
				t.Fatal(err)
			}
			if stream, err := session.OpenStream(); err == nil {
				const N = 100
				var sent string
				for i := 0; i < N; i++ {
					msg := fmt.Sprintf("hello%v", i)
					if _, err := stream.Write([]byte(msg)); err != nil {
						t.Fatal(err)
					}
					sent += msg
				}
				stream.Close()
				sentCh <- sent
				close(sentCh)
			} else {
				t.Fatal(err)
			}
		} else {
			t.Fatal(err)
		}
	}()

	cli, err := net.Dial("tcp", "127.0.0.1:39988")
	if err != nil {
		t.Fatal(err)
	}
	if session, err := newTestClient(cli); err == nil {
		if stream, err := session.AcceptStream(); err == nil {
			buf := make([]byte, 65536)
			var received string
			for {
				n, err := stream.Read(buf)
				if err != nil {
					break
				}
				received += string(buf[:n])
			}
			sent := <-sentCh
			if sent != received {
				t.Fatalf("Expected sent '%s' to equal received: '%s'\n", sent, received)
			}

		} else {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
}

func TestEncryptedClientSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:39978")
	if err != nil {
		// handle error
		t.Fatal(err)
	}
	sentCh := make(chan string)
	go func() {
		if conn, err := ln.Accept(); err == nil {
			session, err := newTestClient(conn)
			if err != nil {
				t.Fatal(err)
			}
			if stream, err := session.OpenStream(); err == nil {
				const N = 100
				var sent string
				for i := 0; i < N; i++ {
					msg := fmt.Sprintf("hello%v", i)
					if _, err := stream.Write([]byte(msg)); err != nil {
						t.Fatal(err)
					}
					sent += msg
				}
				stream.Close()
				sentCh <- sent
				close(sentCh)
			} else {
				t.Fatal(err)
			}
		} else {
			t.Fatal(err)
		}
	}()

	cli, err := net.Dial("tcp", "127.0.0.1:39978")
	if err != nil {
		t.Fatal(err)
	}
	if session, err := newTestServer(cli); err == nil {
		if stream, err := session.AcceptStream(); err == nil {
			buf := make([]byte, 65536)
			var received string
			for {
				n, err := stream.Read(buf)
				if err != nil {
					break
				}
				received += string(buf[:n])
			}
			sent := <-sentCh
			if sent != received {
				t.Fatalf("Expected sent '%s' to equal received: '%s'\n", sent, received)
			}

		} else {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
}

func TestEncryptedSendWithoutRecv(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := session.OpenStream()
	const N = 100
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		stream.Write([]byte(msg))
	}
	buf := make([]byte, 1)
	if _, err := stream.Read(buf); err != nil {
		t.Fatal(err)
	}
	stream.Close()
}

func TestEncryptedWriteAfterClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if _, err := stream.Write([]byte("write after close")); err == nil {
		t.Fatal("write after close failed")
	}
}

func TestEncryptedReadStreamAfterSessionClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := session.OpenStream()
	session.Close()
	buf := make([]byte, 10)
	if _, err := stream.Read(buf); err != nil {
		t.Log(err)
	} else {
		t.Fatal("read stream after session close succeeded")
	}
}

func TestEncryptedNumStreamAfterClose(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.OpenStream(); err == nil {
		if session.NumStreams() != 1 {
			t.Fatal("wrong number of streams after opened")
		}
		session.Close()
		if session.NumStreams() != 0 {
			t.Fatal("wrong number of streams after session closed")
		}
	} else {
		t.Fatal(err)
	}
	cli.Close()
}

func TestEncryptedRandomFrame(t *testing.T) {
	// pure random
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		rnd := make([]byte, rand.Uint32()%1024)
		io.ReadFull(crand.Reader, rnd)
		session.conn.Write(rnd)
	}
	cli.Close()

	// double syn
	cli, err = net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err = newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		f := newFrame(cmdSYN, 1000)
		session.writeFrame(f)
	}
	cli.Close()

	// random cmds
	cli, err = net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	allcmds := []byte{cmdSYN, cmdRST, cmdPSH, cmdNOP}
	session, err = newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		f := newFrame(allcmds[rand.Int()%len(allcmds)], rand.Uint32())
		session.writeFrame(f)
	}
	cli.Close()

	// random cmds & sids
	cli, err = net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err = newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		f := newFrame(byte(rand.Uint32()), rand.Uint32())
		session.writeFrame(f)
	}
	cli.Close()

	// random version
	cli, err = net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err = newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		f := newFrame(byte(rand.Uint32()), rand.Uint32())
		f.ver = byte(rand.Uint32())
		session.writeFrame(f)
	}
	cli.Close()

	// incorrect size
	cli, err = net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err = newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}

	f := newFrame(byte(rand.Uint32()), rand.Uint32())
	rnd := make([]byte, rand.Uint32()%1024)
	io.ReadFull(crand.Reader, rnd)
	f.data = rnd

	buf := make([]byte, headerSize+len(f.data))
	buf[0] = f.ver
	buf[1] = f.cmd
	binary.LittleEndian.PutUint16(buf[2:], uint16(len(rnd)+1)) /// incorrect size
	binary.LittleEndian.PutUint32(buf[4:], f.sid)
	copy(buf[headerSize:], f.data)

	session.conn.Write(buf)
	t.Log(rawHeader(buf))
	cli.Close()
}

func TestEncryptedReadDeadline(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	const N = 100
	buf := make([]byte, 10)
	var readErr error
	for i := 0; i < N; i++ {
		msg := fmt.Sprintf("hello%v", i)
		stream.Write([]byte(msg))
		stream.SetReadDeadline(time.Now().Add(-1 * time.Minute))
		if _, readErr = stream.Read(buf); readErr != nil {
			break
		}
	}
	if readErr != nil {
		if !strings.Contains(readErr.Error(), "i/o timeout") {
			t.Fatalf("Wrong error: %v", readErr)
		}
	} else {
		t.Fatal("No error when reading with past deadline")
	}
	session.Close()
}

func TestEncryptedWriteDeadline(t *testing.T) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStream()
	if err != nil {
		t.Fatal(err)
	}
	const N = 100
	buf := make([]byte, 10)
	var writeErr error
	for i := 0; i < N; i++ {
		stream.SetWriteDeadline(time.Now().Add(-1 * time.Minute))
		if _, writeErr = stream.Write(buf); writeErr != nil {
			break
		}
	}
	if writeErr != nil {
		if !strings.Contains(writeErr.Error(), "i/o timeout") {
			t.Fatalf("Wrong error: %v", writeErr)
		}
	} else {
		t.Fatal("No error when writing with past deadline")
	}
	session.Close()
}

func BenchmarkEncryptedAcceptClose(b *testing.B) {
	cli, err := net.Dial("tcp", "127.0.0.1:19998")
	if err != nil {
		b.Fatal(err)
	}
	session, err := newTestClient(cli)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		if stream, err := session.OpenStream(); err == nil {
			stream.Close()
		} else {
			b.Fatal(err)
		}
	}
}
func BenchmarkEncryptedConnSmux(b *testing.B) {
	cs, ss, err := getEncryptedSmuxStreamPair()
	if err != nil {
		b.Fatal(err)
	}
	defer cs.Close()
	defer ss.Close()
	bench(b, cs, ss)
}

func getEncryptedSmuxStreamPair() (*Stream, *Stream, error) {
	c1, c2, err := getTCPConnectionPair()
	if err != nil {
		return nil, nil, err
	}

	s, err := newTestServer(c2)
	if err != nil {
		return nil, nil, err
	}
	c, err := newTestClient(c1)
	if err != nil {
		return nil, nil, err
	}
	var ss *Stream
	done := make(chan error)
	go func() {
		var rerr error
		ss, rerr = s.AcceptStream()
		done <- rerr
		close(done)
	}()
	cs, err := c.OpenStream()
	if err != nil {
		return nil, nil, err
	}
	err = <-done
	if err != nil {
		return nil, nil, err
	}

	return cs, ss, nil
}
