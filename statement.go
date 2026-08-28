package db2

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-db2/go-db2/network"
	"github.com/go-db2/go-db2/types"
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

// NumInput returns the number of placeholder parameters.
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
	type outInfo struct {
		index int
		dest  any
	}
	var outTargets []outInfo

	for i, arg := range args {
		switch v := arg.Value.(type) {
		case sql.Out:
			outTargets = append(outTargets, outInfo{index: i, dest: v.Dest})
			if v.Dest != nil {
				rawArgs[i] = reflect.ValueOf(v.Dest).Elem().Interface()
			} else {
				rawArgs[i] = 0
			}
		case *sql.Out:
			if v != nil {
				outTargets = append(outTargets, outInfo{index: i, dest: v.Dest})
				if v.Dest != nil {
					rawArgs[i] = reflect.ValueOf(v.Dest).Elem().Interface()
				} else {
					rawArgs[i] = 0
				}
			} else {
				rawArgs[i] = 0
			}
		default:
			rawArgs[i] = arg.Value
		}
	}

	if hasBlobParams(s.paramCols, rawArgs) {
		newQuery, _, newArgs := rewriteBinaryParams(s.query, s.paramCols, rawArgs)
		if len(newArgs) == 0 {
			affected, err := s.conn.session.ExecDirect(ctx, newQuery)
			if err != nil {
				return nil, err
			}
			return NewResult(affected, 0), nil
		}
		_, newParamCols, err := s.conn.session.PrepareAndDescribe(ctx, newQuery)
		if err != nil {
			return nil, err
		}
		affected, outValues, err := s.conn.session.ExecWithParams(ctx, newParamCols, newArgs)
		if err != nil {
			return nil, err
		}
		if len(outTargets) > 0 && len(outValues) > 0 {
			for _, target := range outTargets {
				if target.index < len(outValues) {
					if err := assignOutParam(target.dest, outValues[target.index]); err != nil {
						return nil, err
					}
				}
			}
		}
		return NewResult(affected, 0), nil
	}

	_, newParamCols, err := s.conn.session.PrepareAndDescribe(ctx, s.query)
	if err != nil {
		return nil, err
	}

	affected, outValues, err := s.conn.session.ExecWithParams(ctx, newParamCols, rawArgs)
	if err != nil {
		return nil, err
	}

	if len(outTargets) > 0 && len(outValues) > 0 {
		for _, target := range outTargets {
			if target.index < len(outValues) {
				if err := assignOutParam(target.dest, outValues[target.index]); err != nil {
					return nil, err
				}
			}
		}
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

	trimmed := strings.TrimSpace(strings.ToUpper(s.query))
	if strings.HasPrefix(trimmed, "CALL") {
		_, err := s.ExecContext(ctx, args)
		if err != nil {
			return nil, err
		}
		return NewRows(nil, nil), nil
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

func hasBlobParams(paramCols []network.ColumnDescription, args []any) bool {
	for i, c := range paramCols {
		if (types.SQLType(c.SQLType) == types.SQLTypeBlob || types.SQLType(c.SQLType) == types.SQLTypeNBlob) && i < len(args) {
			if b, ok := args[i].([]byte); ok && len(b) > 0 {
				return true
			}
		}
	}
	return false
}

func rewriteBinaryParams(query string, paramCols []network.ColumnDescription, args []any) (string, []network.ColumnDescription, []any) {
	var rewrittenQuery strings.Builder
	var rewrittenCols []network.ColumnDescription
	var rewrittenArgs []any

	paramIdx := 0
	inString := false

	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			rewrittenQuery.WriteByte(ch)
			if inString && i+1 < len(query) && query[i+1] == '\'' {
				rewrittenQuery.WriteByte(query[i+1])
				i++
				continue
			}
			inString = !inString
		} else if ch == '?' && !inString {
			if paramIdx < len(paramCols) && paramIdx < len(args) {
				c := paramCols[paramIdx]
				if types.SQLType(c.SQLType) == types.SQLTypeBlob || types.SQLType(c.SQLType) == types.SQLTypeNBlob {
					if b, ok := args[paramIdx].([]byte); ok && len(b) > 0 {
						rewrittenQuery.WriteString(fmt.Sprintf("BLOB(X'%x')", b))
						paramIdx++
						continue
					}
				}
				rewrittenQuery.WriteByte(ch)
				rewrittenCols = append(rewrittenCols, paramCols[paramIdx])
				rewrittenArgs = append(rewrittenArgs, args[paramIdx])
				paramIdx++
			} else {
				rewrittenQuery.WriteByte(ch)
			}
		} else {
			rewrittenQuery.WriteByte(ch)
		}
	}

	return rewrittenQuery.String(), rewrittenCols, rewrittenArgs
}

func assignOutParam(dest any, val any) error {
	if dest == nil || val == nil {
		return nil
	}
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.IsNil() {
		return fmt.Errorf("db2: sql.Out destination must be a non-nil pointer, got %T", dest)
	}
	elem := destVal.Elem()
	valVal := reflect.ValueOf(val)

	if valVal.Type().AssignableTo(elem.Type()) {
		elem.Set(valVal)
		return nil
	}
	if valVal.Type().ConvertibleTo(elem.Type()) {
		elem.Set(valVal.Convert(elem.Type()))
		return nil
	}

	// String fallback
	if elem.Kind() == reflect.String {
		elem.SetString(fmt.Sprint(val))
		return nil
	}

	return fmt.Errorf("db2: cannot assign value %v (%T) to output parameter %v (%T)", val, val, elem.Interface(), elem.Type())
}

// CheckNamedValue implements driver.NamedValueChecker interface.
func (s *Stmt) CheckNamedValue(nv *driver.NamedValue) error {
	return nil
}

var (
	_ driver.Stmt              = (*Stmt)(nil)
	_ driver.StmtExecContext   = (*Stmt)(nil)
	_ driver.StmtQueryContext  = (*Stmt)(nil)
	_ driver.NamedValueChecker = (*Stmt)(nil)
)
