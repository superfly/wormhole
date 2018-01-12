package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	log "github.com/sirupsen/logrus"

	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/tlstest"
	"github.com/superfly/wormhole/messages"
	"github.com/superfly/wormhole/session"
)

func TestTLSConfig_BadCert(t *testing.T) {
	registry := session.NewRegistry(log.New())
	tlsc, err := NewConfig([]byte{}, []byte{}, registry)
	assert.Error(t, err, "Should raise error when key pair is invalid")
	assert.Nil(t, tlsc, "Should be nil when key pair is invalid")
}

func TestTLSConfig_GoodCert(t *testing.T) {
	registry := session.NewRegistry(log.New())
	serverCrtPEM, serverKeyPEM, err := tlstest.CreateRootCertKeyPEMPair()
	if err != nil {
		t.Fatal("couldn't generate a key pair: ", err)
	}

	tlsc, err := NewConfig(serverCrtPEM, serverKeyPEM, registry)
	assert.NoError(t, err, "Should raise error when key pair is invalid")
	assert.NotNil(t, tlsc, "Should be nil when key pair is invalid")
}

func TestTLSConfig_DefaultConfig(t *testing.T) {
	registry := session.NewRegistry(log.New())
	serverCrtPEM, serverKeyPEM, err := tlstest.CreateRootCertKeyPEMPair()
	if err != nil {
		t.Fatal("couldn't generate a key pair: ", err)
	}

	tlsc, err := NewConfig(serverCrtPEM, serverKeyPEM, registry)
	if err != nil {
		t.Fatal("unexpected tls.Config error: ", err)
	}

	cfg := tlsc.GetDefaultConfig()

	assert.Equal(t, 1, len(cfg.Certificates))

	keypair, _ := tls.X509KeyPair(serverCrtPEM, serverKeyPEM)
	assert.Equal(t, keypair, cfg.Certificates[0])

	assert.Equal(t, []tls.CurveID{tls.CurveP256, tls.X25519}, cfg.CurvePreferences)

	assert.True(t, cfg.PreferServerCipherSuites)
	assert.NotNil(t, cfg.GetConfigForClient)
	assert.Nil(t, cfg.VerifyPeerCertificate)
	assert.Nil(t, cfg.GetCertificate)
}

func TestTLSConfig_GetConfigForClient(t *testing.T) {
	registry := session.NewRegistry(log.New())
	rootPEM, certPEM, keyPEM, err := tlstest.CreateServerCertKeyPEMPairWithRootCert()
	if err != nil {
		t.Fatal("couldn't generate a key pair: ", err)
	}

	tlsc, err := NewConfig(certPEM, keyPEM, registry)
	if err != nil {
		t.Fatal("unexpected tls.Config error: ", err)
	}

	cfg := tlsc.GetDefaultConfig()

	clientCfg, err := cfg.GetConfigForClient(&tls.ClientHelloInfo{})
	assert.Error(t, err)
	assert.Nil(t, clientCfg)

	clientCfg, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "badname"})
	assert.Error(t, err)
	assert.Nil(t, clientCfg)

	clientCfg, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "idnotfound.wormhole.test"})
	assert.Error(t, err)
	assert.Nil(t, clientCfg)

	noClientAuth := &testSession{
		id: "no-client-auth",
	}
	registry.AddSession(noClientAuth)

	clientCfg, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "no-client-auth.wormhole.test"})
	assert.NoError(t, err)
	assert.NotNil(t, clientCfg)
	diff := deep.Equal(clientCfg, tlsc.GetDefaultConfig())
	assert.Nil(t, diff, "session with no client auth should receive default TLS config")

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(rootPEM)
	tlscert, _ := tls.X509KeyPair(certPEM, keyPEM)
	cert, _ := x509.ParseCertificate(tlscert.Certificate[0])
	clientAuth := &testSession{
		id:                "client-auth",
		clientAuthEnabled: true,
		certPool:          pool,
		validCert:         cert,
	}
	registry.AddSession(clientAuth)

	clientCfg, err = cfg.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "client-auth.wormhole.test"})
	assert.NoError(t, err)
	assert.NotNil(t, clientCfg)
	assert.Equal(t, tls.RequireAndVerifyClientCert, clientCfg.ClientAuth)
	assert.Equal(t, pool, clientCfg.ClientCAs)
}

type testSession struct {
	id                string
	clientAuthEnabled bool
	validCert         *x509.Certificate
	certPool          *x509.CertPool
}

func (ts *testSession) ID() string {
	return ts.id
}

func (ts *testSession) Agent() string {
	return ""
}

func (ts *testSession) BackendID() string {
	return ""
}

func (ts *testSession) NodeID() string {
	return ""
}

func (ts *testSession) Client() string {
	return ""
}

func (ts *testSession) ClientIP() string {
	return ""
}

func (ts *testSession) Cluster() string {
	return ""
}

func (ts *testSession) Endpoints() []net.Addr {
	return []net.Addr{}
}

func (ts *testSession) AddEndpoint(endpoint net.Addr) {
}

func (ts *testSession) Key() string {
	return ""
}

func (ts *testSession) Release() *messages.Release {
	return nil
}

func (ts *testSession) RequireStream() error {
	return errors.New("not implemented")
}

func (ts *testSession) RequireAuthentication() error {
	return errors.New("not implemented")
}

func (ts *testSession) RequiresClientAuth() bool {
	return ts.clientAuthEnabled
}
func (ts *testSession) ClientCAs() (*x509.CertPool, error) {
	if ts.certPool != nil {
		return ts.certPool, nil
	}
	return nil, errors.New("not set-up")
}

func (ts *testSession) ValidCertificate(c *x509.Certificate) (bool, error) {
	if ts.validCert == nil {
		return false, errors.New("not set-up")
	}

	return ts.validCert.Equal(c), nil
}

func (ts *testSession) Close() {

}
