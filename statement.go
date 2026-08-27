package db2

import (
	"context"
	"database/sql/driver"
	"fmt"

	"github.com/go-db2/go-db2/network"
)

// Stmt implements the database/sql/driver.Stmt and StmtExecContext / StmtQueryContext interfaces.
type Stmt struct {
	conn       *Conn
	query      string
	outputCols []network.ColumnDescription
	paramCols  []network.ColumnDescription
	closed     bool
}

// NewStmt creates a prepared statement wrapper.
func NewStmt(conn *Conn, query string, outputCols, paramCols []network.ColumnDescription) *Stmt {
	return &Stmt{
		conn:       conn,
		query:      query,
		outputCols: outputCols,
		paramCols:  paramCols,
	}
}

// Close closes the prepared statement.
func (s *Stmt) Close() error {
	s.closed = true
	return nil
}

// NumInput returns the number of placeholder parameters expected by the statement.
func (s *Stmt) NumInput() int {
	return len(s.paramCols)
}

// Exec executes a prepared statement with positional arguments (legacy interface).
func (s *Stmt) Exec(args []driver.Value) (driver.Result, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.ExecContext(context.Background(), namedArgs)
}

// ExecContext executes a prepared statement with context and arguments.
func (s *Stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.closed || s.conn == nil || s.conn.session == nil {
		return nil, ErrConnectionClosed
	}

	if len(args) != len(s.paramCols) {
		return nil, fmt.Errorf("db2: expected %d arguments, got %d", len(s.paramCols), len(args))
	}

	rawArgs := make([]any, len(args))
	for i, arg := range args {
		rawArgs[i] = arg.Value
	}

	affected, err := s.conn.session.ExecWithParams(ctx, s.paramCols, rawArgs)
	if err != nil {
		return nil, err
	}

	return NewResult(affected, 0), nil
}

// Query executes a prepared query statement with positional arguments (legacy interface).
func (s *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	namedArgs := make([]driver.NamedValue, len(args))
	for i, v := range args {
		namedArgs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.QueryContext(context.Background(), namedArgs)
}

// QueryContext executes a prepared query statement with context and arguments.
func (s *Stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.closed || s.conn == nil || s.conn.session == nil {
		return nil, ErrConnectionClosed
	}

	if len(args) != len(s.paramCols) {
		return nil, fmt.Errorf("db2: expected %d arguments, got %d", len(s.paramCols), len(args))
	}

	rawArgs := make([]any, len(args))
	for i, arg := range args {
		rawArgs[i] = arg.Value
	}

	cols, rawRows, err := s.conn.session.QueryWithParams(ctx, s.outputCols, s.paramCols, rawArgs)
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

var (
	_ driver.Stmt             = (*Stmt)(nil)
	_ driver.StmtExecContext  = (*Stmt)(nil)
	_ driver.StmtQueryContext = (*Stmt)(nil)
)
