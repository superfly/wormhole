package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	bugsnag_hook "github.com/Shopify/logrus-bugsnag"
	"github.com/Sirupsen/logrus"
	bugsnag "github.com/bugsnag/bugsnag-go"
	"github.com/spf13/viper"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

const (
	unsetEnvStr = "%s needs to be set"
	invalidStr  = "%s is invalid"
)

var (
	// ErrInvalidConfig when missing or wrong config values
	ErrInvalidConfig = errors.New("config is invalid")

	// populated by build server
	version string
)

func init() {
	//set common defaults for server and client config

	viper.SetDefault("proto", "ssh")
	viper.SetDefault("log_level", "info")

	// only set it when version is provided by the build system
	if len(version) > 0 {
		viper.Set("version", version)
	} else {
		viper.Set("version", "latest")
	}

	viper.SetConfigName("wormhole") // name of config file (without extension)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".") // path to look for the config file in
	viper.ReadInConfig()     // Find and read the config file

	viper.SetEnvPrefix("fly")
	viper.AutomaticEnv()
}

// Version returns version of wormhole
func Version() string {
	return viper.GetString("version")
}

// Config stores wormole shared parameters
type Config struct {
	// Protocol specifies transportation layer used by wormhole
	// e.g. SSH tunneling, TLS conn pool, etc.
	Protocol TunnelProto

	// for server this means its listening port
	// for client this means listening port of the local server
	Port string

	// wormhole's version
	Version string

	// for server this means hostname or IP address of the host/container running
	// a particular server instance
	// for client this means the hostname or IP address of the local server
	Localhost string

	// TLS cert is used when TLS conn pool is used as transporation layer
	// Server also needs TLSPrivateKey
	// Client should only need a cert if the cert is not verifiable using system Root CAs
	TLSCert []byte

	// Logging level
	LogLevel string

	// Logger instance
	Logger *logrus.Logger
}

// ServerConfig stores wormhole server parameters
type ServerConfig struct {
	Config

	// cluster identifier of wormhole servers
	// used as metadata for session storage
	ClusterURL string

	// URL of Redis instance
	// Redis powers the session storage
	RedisURL string

	// ID of the wormhole server
	// used as metadata for session storage
	NodeID string

	// SSH private key is used by the server when SSH tunneling is used
	// as transportation layer
	SSHPrivateKey []byte

	// TLS Private key is used by the server when TLS conn pool is used
	// as transporation layer
	TLSPrivateKey []byte

	// API token for error reporting to Bugsnag
	BugsnagAPIKey string

	// Port used by HTTP server to serve metrics
	// Used by Prometheus to scrape wormhole server endpoint
	MetricsAPIPort string
}

// NewServerConfig parses config values collected from Viper and validates them
// it returns a ServerConfig struct or an error if config values are invalid
func NewServerConfig() (*ServerConfig, error) {
	viper.SetDefault("localhost", os.Getenv("IPADDRESS"))
	nodeID, _ := os.Hostname()
	viper.SetDefault("node_id", nodeID)
	viper.SetDefault("port", "10000")
	viper.SetDefault("metrics_api_port", "9191")
	viper.BindEnv("bugsnag_api_key", "BUGSNAG_API_KEY")

	logger := logrus.New()
	logger.Formatter = new(prefixed.TextFormatter)
	logger.Level = parseLogLevel(viper.GetString("log_level"))

	version := viper.GetString("version")
	bugsnagKey := viper.GetString("bugsnag_api_key")
	if len(bugsnagKey) > 0 {
		bugsnag.Configure(bugsnag.Configuration{
			APIKey:     bugsnagKey,
			AppVersion: version,
		})
		hook, err := bugsnag_hook.NewBugsnagHook()
		if err != nil {
			return nil, cfgErr(invalidStr, "BUGSNAG_API_KEY")
		}
		logger.Hooks.Add(hook)
	}

	protocol := ParseTunnelProto(viper.GetString("proto"))

	shared := Config{
		Protocol:  protocol,
		Port:      viper.GetString("port"),
		Version:   version,
		Localhost: viper.GetString("localhost"),
		LogLevel:  viper.GetString("log_level"),
		Logger:    logger,
	}

	cfg := &ServerConfig{
		ClusterURL:     viper.GetString("cluster_url"),
		RedisURL:       viper.GetString("redis_url"),
		NodeID:         viper.GetString("node_id"),
		MetricsAPIPort: viper.GetString("metrics_api_port"),
		Config:         shared,
	}

	switch protocol {
	case SSH:
		sshKey, err := ioutil.ReadFile(viper.GetString("ssh_private_key_file"))
		if err != nil {
			return nil, cfgErr(unsetEnvStr, "FLY_SSH_PRIVATE_KEY_FILE")
		}
		cfg.SSHPrivateKey = sshKey
	case TLS:
		tlsKey, err := ioutil.ReadFile(viper.GetString("tls_private_key_file"))
		if err != nil {
			return nil, cfgErr(unsetEnvStr, "FLY_TLS_PRIVATE_KEY_FILE")
		}
		cfg.TLSPrivateKey = tlsKey

		tlsCert, err := ioutil.ReadFile(viper.GetString("tls_cert_file"))
		if err != nil {
			return nil, cfgErr(unsetEnvStr, "FLY_TLS_CERT_FILE")
		}
		cfg.TLSCert = tlsCert
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *ServerConfig) validate() error {
	switch cfg.Protocol {
	case UNSUPPORTED:
		return cfgErr(unsetEnvStr, "FLY_PROTO")
	case TLS:
		if len(cfg.TLSCert) == 0 {
			return cfgErr(invalidStr, "FLY_TLS_CERT_FILE")
		} else if len(cfg.TLSPrivateKey) == 0 {
			return cfgErr(invalidStr, "FLY_TLS_PRIVATE_KEY_FILE")
		}
	case SSH:
		if len(cfg.SSHPrivateKey) == 0 {
			return cfgErr(invalidStr, "FLY_SSH_PRIVATE_KEY_FILE")
		}
	}

	if len(cfg.Port) == 0 {
		return cfgErr(unsetEnvStr, "FLY_PORT")
	} else if len(cfg.Localhost) == 0 {
		return cfgErr(unsetEnvStr, "FLY_LOCALHOST or IPADDRESS")
	} else if len(cfg.LogLevel) == 0 {
		return cfgErr(unsetEnvStr, "FLY_LOG_LEVEL")
	} else if len(cfg.ClusterURL) == 0 {
		return cfgErr(unsetEnvStr, "FLY_CLUSTER_URL")
	} else if len(cfg.RedisURL) == 0 {
		return cfgErr(unsetEnvStr, "FLY_REDIS_URL")
	} else if len(cfg.NodeID) == 0 {
		return cfgErr(unsetEnvStr, "FLY_NODE_ID")
	} else if len(cfg.MetricsAPIPort) == 0 {
		return cfgErr(unsetEnvStr, "FLY_METRICS_API_PORT")
	}
	return nil
}

// ClientConfig stores wormhole client parameters
type ClientConfig struct {
	Config

	// <HOST>:<PORT> of the user's server (e.g. Rails server)
	LocalEndpoint string

	// <HOST>:<PORT> of the wormhole server
	RemoteEndpoint string

	// Authentication token when connecting to wormhole server
	Token string

	// when set this will override the default VCS ID (i.e. git commit SHA1)
	// defaults to FLY_RELASE_ID (but can be overriden with FLY_RELEASE_ID_VAR to point ot a different ENV)
	ReleaseID string

	// when set this will override the default VCS message (i.e. git commit message)
	// defaults to FLY_RELASE_DESC (but can be overriden with FLY_RELEASE_DESC_VAR to point ot a different ENV)
	ReleaseDesc string

	// when set this will override the default VCS branch
	// defaults to FLY_RELASE_BRANCH (but can be overriden with FLY_RELEASE_BRANCH_VAR to point ot a different ENV)
	ReleaseBranch string
}

// NewClientConfig parses config values collected from Viper and validates them
// it returns a ClientConfig struct or an error if config values are invalid
func NewClientConfig() (*ClientConfig, error) {
	viper.SetDefault("port", "5000")
	viper.SetDefault("localhost", "127.0.0.1")
	viper.SetDefault("remote_endpoint", "wormhole.fly.io:30000")
	viper.SetDefault("local_endpoint", viper.GetString("localhost")+":"+viper.GetString("port"))
	viper.SetDefault("release_id_var", "FLY_RELEASE_ID")
	viper.SetDefault("release_desc_var", "FLY_RELEASE_DESC")
	viper.SetDefault("release_branch_var", "FLY_RELEASE_BRANCH")

	logger := logrus.New()
	logger.Formatter = new(prefixed.TextFormatter)
	logger.Level = parseLogLevel(viper.GetString("log_level"))

	protocol := ParseTunnelProto(viper.GetString("proto"))

	shared := Config{
		Protocol:  protocol,
		Port:      viper.GetString("port"),
		Version:   viper.GetString("version"),
		Localhost: viper.GetString("localhost"),
		LogLevel:  viper.GetString("log_level"),
		Logger:    logger,
	}

	if protocol == TLS {
		tlsCert, err := ioutil.ReadFile(viper.GetString("tls_cert_file"))
		if err != nil {
			return nil, cfgErr(unsetEnvStr, "FLY_TLS_CERT_FILE")
		}
		shared.TLSCert = tlsCert
	}

	cfg := &ClientConfig{
		LocalEndpoint:  viper.GetString("local_endpoint"),
		RemoteEndpoint: viper.GetString("remote_endpoint"),
		Token:          viper.GetString("token"),
		ReleaseID:      os.Getenv(viper.GetString("release_id_var")),
		ReleaseBranch:  os.Getenv(viper.GetString("release_branch_var")),
		ReleaseDesc:    os.Getenv(viper.GetString("release_desc_var")),
		Config:         shared,
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *ClientConfig) validate() error {
	switch cfg.Protocol {
	case UNSUPPORTED:
		return cfgErr(unsetEnvStr, "FLY_PROTO")
	case TLS:
		if len(cfg.TLSCert) == 0 {
			return cfgErr(invalidStr, "FLY_TLS_CERT_KEY_FILE")
		}
	}

	if len(cfg.Port) == 0 {
		return cfgErr(unsetEnvStr, "FLY_PORT")
	} else if len(cfg.Localhost) == 0 {
		return cfgErr(unsetEnvStr, "FLY_LOCALHOST")
	} else if len(cfg.LogLevel) == 0 {
		return cfgErr(unsetEnvStr, "FLY_LOG_LEVEL")
	} else if len(cfg.LocalEndpoint) == 0 {
		return cfgErr(unsetEnvStr, "FLY_LOCAL_ENDPOINT")
	} else if len(cfg.RemoteEndpoint) == 0 {
		return cfgErr(unsetEnvStr, "FLY_REMOTE_ENDPOINT")
	} else if len(cfg.Token) == 0 {
		return cfgErr(unsetEnvStr, "FLY_TOKEN")
	}
	return nil
}

func parseLogLevel(lvl string) logrus.Level {
	level, err := logrus.ParseLevel(lvl)
	if err != nil {
		return logrus.InfoLevel
	}
	return level
}

func cfgErr(template string, vars ...interface{}) error {
	return fmt.Errorf(template, vars)
}

// TunnelProto specifies the type of transport protocol used by wormhole instance
type TunnelProto int

const (
	// SSH tunnel with remote port forwarding
	SSH TunnelProto = iota
	// TCP connection pool
	TCP
	// TLS connection pool
	TLS
	_
	_
	_
	// UNSUPPORTED is a catch all for unsupported protocol types
	UNSUPPORTED
)

// ParseTunnelProto converts protocol string name to TunnelProto
func ParseTunnelProto(proto string) TunnelProto {
	switch proto {
	case "ssh":
		return SSH
	case "tcp":
		return TCP
	case "tls":
		return TLS
	default:
		return UNSUPPORTED
	}
}
