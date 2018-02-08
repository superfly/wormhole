package session

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis"
	"github.com/garyburd/redigo/redis"
	"github.com/superfly/tlstest"
)

var (
	redisPool       *redis.Pool
	serverTLSConfig *tls.Config
	clientTLSConfig *tls.Config

	serverTLSCert tls.Certificate
	serverCrtPEM  []byte
	serverKeyPEM  []byte

	testRedis *miniredis.Miniredis
)

// One place to define setup and teardown for all tests in session package
func TestMain(m *testing.M) {
	var rootCrtPEM []byte
	var err error
	rootCrtPEM, serverCrtPEM, serverKeyPEM, err = tlstest.CreateServerCertKeyPEMPairWithRootCert()
	if err != nil {
		log.Fatalf("tlstest could not generate x509 certs %v+", err)
	}

	serverTLSCert, err = tls.X509KeyPair(serverCrtPEM, serverKeyPEM)
	if err != nil {
		log.Fatalf("Couldn't create tls cert from keypair %v+", err)
	}

	serverTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{serverTLSCert},
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(rootCrtPEM)

	clientTLSConfig = &tls.Config{
		RootCAs:    certPool,
		ServerName: "127.0.0.1",
	}

	testRedis, err = miniredis.Run()
	if err != nil {
		log.Fatalf("Couldn't create miniredis instance %v+", err)
	}
	defer testRedis.Close()

	redisPool = newTestRedisPool(testRedis.Addr())
	defer redisPool.Close()

	code := m.Run()

	os.Exit(code)
}

func newTestRedisPool(hostPort string) *redis.Pool {
	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", hostPort)
		},
		TestOnBorrow: func(conn redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := conn.Do("PING")
			return err
		},
	}
}
