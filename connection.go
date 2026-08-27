package db2

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
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

	return nil, errors.New("db2: prepared statements with parameters will be implemented in Phase 3")
}

// Exec executes a query that doesn't return rows (legacy interface).
func (c *Conn) Exec(query string, args []driver.Value) (driver.Result, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return c.ExecContext(context.Background(), query, namedArgs)
}

// ExecContext executes a query without returning rows.
func (c *Conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.session == nil {
		return nil, ErrConnectionClosed
	}

	if len(args) > 0 {
		return nil, fmt.Errorf("db2: parameter binding in direct Exec is not supported yet (args count=%d)", len(args))
	}

	affected, err := c.session.ExecDirect(ctx, query)
	if err != nil {
		return nil, err
	}

	return NewResult(affected, 0), nil
}

// Query executes a query that may return rows (legacy interface).
func (c *Conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return c.QueryContext(context.Background(), query, namedArgs)
}

// QueryContext executes a query that may return rows.
func (c *Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.session == nil {
		return nil, ErrConnectionClosed
	}

	if len(args) > 0 {
		return nil, fmt.Errorf("db2: parameter binding in direct Query is not supported yet (args count=%d)", len(args))
	}

	cols, rawRows, err := c.session.QueryDirect(ctx, query)
	if err != nil {
		return nil, err
	}

	rowsData := make([][]driver.Value, len(rawRows))
	for i, r := range rawRows {
		row := make([]driver.Value, len(r))
		for j, v := range r {
			row[j] = driver.Value(v)
		}
		rowsData[i] = row
	}

	return NewRows(cols, rowsData), nil
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
	_ driver.ExecerContext      = (*Conn)(nil)
	_ driver.QueryerContext     = (*Conn)(nil)
)
