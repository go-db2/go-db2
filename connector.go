package db2

import (
	"context"
	"database/sql/driver"
	"errors"
	"maps"

	"github.com/go-db2/go-db2/internal/drda"
)

// Connector implements the database/sql/driver.Connector interface.
type Connector struct {
	cfg    *Config
	driver driver.Driver
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return NewConfig()
	}
	copied := *cfg
	if cfg.TLSConfig != nil {
		copied.TLSConfig = cfg.TLSConfig.Clone()
	}
	if cfg.Params != nil {
		copied.Params = maps.Clone(cfg.Params)
	}
	return &copied
}

// NewConnector creates a new Connector from a parsed Config.
// If cfg is nil, default settings from NewConfig are used; otherwise cfg is defensively cloned.
func NewConnector(cfg *Config) *Connector {
	return &Connector{
		cfg:    cloneConfig(cfg),
		driver: &Driver{},
	}
}

// Connect returns a new active connection to the database.
func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	if c == nil || c.cfg == nil {
		return nil, errors.New("db2: nil connector or configuration")
	}
	sessionCfg := c.cfg.toSessionConfig()
	session := drda.NewSession(sessionCfg)

	if err := session.Connect(ctx); err != nil {
		return nil, err
	}

	return newConn(session, c.cfg), nil
}

// Driver returns the underlying Driver of the Connector.
func (c *Connector) Driver() driver.Driver {
	return c.driver
}

var _ driver.Connector = (*Connector)(nil)
