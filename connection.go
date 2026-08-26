package db2

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"

	"github.com/go-db2/go-db2/network"
)

// Conn implements the database/sql/driver.Conn interface for IBM Db2.
type Conn struct {
	session *network.Session
	cfg     *Config
	mu      sync.Mutex
	closed  bool
}

// NewConn wraps a connected network session into a database/sql driver.Conn.
func NewConn(session *network.Session, cfg *Config) *Conn {
	return &Conn{
		session: session,
		cfg:     cfg,
	}
}

// Prepare returns a prepared statement, bound to this connection.
func (c *Conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext returns a prepared statement, bound to this connection with context support.
func (c *Conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.session == nil {
		return nil, ErrConnectionClosed
	}

	// Will be fully wired with Statement in Phase 2
	return nil, errors.New("db2: prepared statements not yet fully implemented in minimal driver")
}

// Close invalidates and closes the database connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// Begin starts and returns a new transaction (legacy interface).
func (c *Conn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts and returns a new transaction with the provided context and options.
func (c *Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.session == nil {
		return nil, ErrConnectionClosed
	}

	if opts.ReadOnly {
		return nil, errors.New("db2: read-only transactions are not supported")
	}

	return &Tx{
		session: c.session,
		ctx:     ctx,
	}, nil
}

// Ping implements driver.Pinger interface to verify connection liveness.
func (c *Conn) Ping(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.session == nil {
		return ErrConnectionClosed
	}

	return c.session.Ping(ctx)
}

// Interface assertions
var (
	_ driver.Conn               = (*Conn)(nil)
	_ driver.ConnPrepareContext = (*Conn)(nil)
	_ driver.ConnBeginTx        = (*Conn)(nil)
	_ driver.Pinger             = (*Conn)(nil)
)
