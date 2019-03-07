package tls

// TLSConfig represents a tls.Config holder that contains a list of default TLS
import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/superfly/wormhole/session"
)

// Config uses session.Registry to generate tls.Config's dynamically for each session.
// E.g. some session will require client cert authentication.
type Config struct {
	cert     *tls.Certificate
	registry *session.Registry
}

// NewConfig returns a new Config with a certificate or an error if
// the default certificate cannot be loaded.
func NewConfig(certPEM, keyPEM []byte, registry *session.Registry) (*Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "could not load default the certificate")
	}

	return &Config{
		cert:     &cert,
		registry: registry,
	}, nil
}

// GetDefaultConfig returns the default tls.Config
func (c *Config) GetDefaultConfig() *tls.Config {
	return &tls.Config{
		Certificates:             []tls.Certificate{*c.cert},
		CurvePreferences:         []tls.CurveID{tls.CurveP256, tls.X25519},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			// prefer ECDSA, because of abysymal RSA performance in Go
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		GetConfigForClient: c.getConfigForClient,
	}
}

func (c *Config) getConfigForClient(helloInfo *tls.ClientHelloInfo) (*tls.Config, error) {

	id := strings.Split(helloInfo.ServerName, ".")[0]
	if len(id) == 0 {
		return nil, fmt.Errorf("SNI has no ID")
	}

	if id == "api" {
		return c.GetDefaultConfig(), nil
	}

	session := c.registry.GetSession(id)
	if session == nil {
		return nil, fmt.Errorf("Session (ID='%s') cannot be found", id)
	}

	cfg := c.GetDefaultConfig()
	if session.RequiresClientAuth() {
		cas, err := session.ClientCAs()
		if err != nil {
			return nil, fmt.Errorf("Couldn't get Client CAs for backend (ID='%s'): %s", session.BackendID(), err.Error())
		}
		cfg.ClientCAs = cas
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.VerifyPeerCertificate = verifyPeerCertificateFunc(session)
	}

	return cfg, nil
}

func verifyPeerCertificateFunc(session session.Session) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(verifiedChains) == 0 {
			return errors.New("no certificate in verified chain")
		}

		for _, chain := range verifiedChains {
			for _, cert := range chain {
				ok, err := session.ValidCertificate(cert)
				if err != nil {
					return fmt.Errorf("Couldn't validate certificate for backend (ID='%s'): %s", session.BackendID(), err.Error())
				}
				if ok {
					return nil
				}
			}
		}
		return fmt.Errorf("Couldn't validate certificate for backend (ID='%s')", session.BackendID())
	}
}
