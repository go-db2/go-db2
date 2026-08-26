package db2

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-db2/go-db2/network"
)

// Config holds the configuration options parsed from a connection string (DSN or URL).
type Config struct {
	Host              string
	Port              int
	Database          string
	User              string
	Password          string
	UseSSL            bool
	SSLClientCertPath string
	Timeout           time.Duration
	Params            map[string]string
}

// NewConfig returns a default Config with standard Db2 settings.
func NewConfig() *Config {
	return &Config{
		Host:    "localhost",
		Port:    50000,
		Timeout: 30 * time.Second,
		Params:  make(map[string]string),
	}
}

// ParseDSN parses a Db2 connection string in either URL format or Key-Value DSN format.
//
// Examples:
//
//	db2://user:password@localhost:50000/sample?ssl=true&timeout=30s
//	host=localhost;port=50000;database=sample;user=db2inst1;password=password;ssl=false;
func ParseDSN(dsn string) (*Config, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, ErrInvalidConnectionStr
	}

	if strings.HasPrefix(dsn, "db2://") || strings.HasPrefix(dsn, "db2s://") {
		return parseURL(dsn)
	}

	return parseKeyValue(dsn)
}

func parseURL(dsn string) (*Config, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to parse URL: %v", ErrInvalidConnectionStr, err)
	}

	cfg := NewConfig()

	if u.Scheme == "db2s" {
		cfg.UseSSL = true
		cfg.Port = 50001
	}

	cfg.Host = u.Hostname()
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}

	if portStr := u.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("%w: invalid port %q", ErrInvalidConnectionStr, portStr)
		}
		cfg.Port = port
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	// Database name is the path without leading slash
	cfg.Database = strings.TrimPrefix(u.Path, "/")

	// Query parameters
	queryParams := u.Query()
	for k, v := range queryParams {
		val := ""
		if len(v) > 0 {
			val = v[0]
		}
		applyParam(cfg, k, val)
	}

	return cfg, nil
}

func parseKeyValue(dsn string) (*Config, error) {
	cfg := NewConfig()
	pairs := strings.Split(dsn, ";")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		applyParam(cfg, key, val)
	}

	return cfg, nil
}

func applyParam(cfg *Config, key, val string) {
	lowerKey := strings.ToLower(key)
	switch lowerKey {
	case "host", "server", "hostname":
		cfg.Host = val
	case "port":
		if port, err := strconv.Atoi(val); err == nil && port > 0 && port <= 65535 {
			cfg.Port = port
		}
	case "database", "dbname", "db":
		cfg.Database = val
	case "user", "uid", "username":
		cfg.User = val
	case "password", "pwd":
		cfg.Password = val
	case "ssl", "use_ssl", "usessl":
		cfg.UseSSL = (strings.ToLower(val) == "true" || val == "1" || strings.ToLower(val) == "yes")
		if cfg.UseSSL && cfg.Port == 50000 {
			cfg.Port = 50001
		}
	case "ssl_client_cert_path", "sslcert", "cert":
		cfg.SSLClientCertPath = val
		cfg.UseSSL = true
	case "timeout", "connect_timeout":
		if d, err := time.ParseDuration(val); err == nil {
			cfg.Timeout = d
		} else if sec, err := strconv.Atoi(val); err == nil {
			cfg.Timeout = time.Duration(sec) * time.Second
		}
	default:
		cfg.Params[key] = val
	}
}

// ToSessionConfig converts Config to the network.SessionConfig structure.
func (c *Config) ToSessionConfig() network.SessionConfig {
	return network.SessionConfig{
		Host:              c.Host,
		Port:              c.Port,
		Database:          c.Database,
		User:              c.User,
		Password:          c.Password,
		UseSSL:            c.UseSSL,
		SSLClientCertPath: c.SSLClientCertPath,
		Timeout:           c.Timeout,
	}
}
