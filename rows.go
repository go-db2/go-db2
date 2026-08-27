package db2

import (
	"database/sql/driver"
	"io"
	"reflect"
	"time"

	"github.com/go-db2/go-db2/network"
	"github.com/go-db2/go-db2/types"
)

// Rows implements database/sql/driver.Rows and advanced column type introspection interfaces.
type Rows struct {
	columns []network.ColumnDescription
	data    [][]driver.Value
	cursor  int
	closed  bool
}

// NewRows instantiates a new driver.Rows container.
func NewRows(columns []network.ColumnDescription, data [][]driver.Value) *Rows {
	return &Rows{
		columns: columns,
		data:    data,
		cursor:  0,
	}
}

// Columns returns the names of the columns.
func (r *Rows) Columns() []string {
	names := make([]string, len(r.columns))
	for i, c := range r.columns {
		names[i] = c.Name
	}
	return names
}

// Close closes the rows iterator.
func (r *Rows) Close() error {
	r.closed = true
	r.data = nil
	return nil
}

// Next is called to populate the next row of data into the provided slice.
func (r *Rows) Next(dest []driver.Value) error {
	if r.closed {
		return io.EOF
	}
	if r.cursor >= len(r.data) {
		return io.EOF
	}

	row := r.data[r.cursor]
	r.cursor++

	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		} else {
			dest[i] = nil
		}
	}

	return nil
}

// ColumnTypeScanType returns the Go type that can be used to scan types into.
func (r *Rows) ColumnTypeScanType(index int) reflect.Type {
	if index < 0 || index >= len(r.columns) {
		return reflect.TypeOf(new(any)).Elem()
	}

	sqlType := types.SQLType(r.columns[index].SQLType).BaseType()
	switch sqlType {
	case types.SQLTypeSmall:
		return reflect.TypeOf(int16(0))
	case types.SQLTypeInteger:
		return reflect.TypeOf(int32(0))
	case types.SQLTypeBigInt:
		return reflect.TypeOf(int64(0))
	case types.SQLTypeFloat:
		return reflect.TypeOf(float64(0))
	case types.SQLTypeDate, types.SQLTypeTimestamp:
		return reflect.TypeOf(time.Time{})
	case types.SQLTypeBlob, types.SQLTypeBinary, types.SQLTypeVarBinary:
		return reflect.TypeOf([]byte{})
	case types.SQLTypeBoolean:
		return reflect.TypeOf(true)
	default:
		return reflect.TypeOf("")
	}
}

// ColumnTypeDatabaseTypeName returns the database system type name.
func (r *Rows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.columns) {
		return ""
	}
	return types.SQLType(r.columns[index].SQLType).BaseType().String()
}

// ColumnTypeNullable returns whether the column is nullable.
func (r *Rows) ColumnTypeNullable(index int) (nullable, ok bool) {
	if index < 0 || index >= len(r.columns) {
		return false, false
	}
	return r.columns[index].Nullable, true
}

// ColumnTypeLength returns the length of the column type if variable length.
func (r *Rows) ColumnTypeLength(index int) (length int64, ok bool) {
	if index < 0 || index >= len(r.columns) {
		return 0, false
	}
	return r.columns[index].Length, true
}

// ColumnTypePrecisionScale returns the precision and scale for decimal and numeric types.
func (r *Rows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	if index < 0 || index >= len(r.columns) {
		return 0, 0, false
	}
	col := r.columns[index]
	if col.Precision > 0 || col.Scale > 0 {
		return int64(col.Precision), int64(col.Scale), true
	}
	return 0, 0, false
}

var (
	_ driver.Rows                           = (*Rows)(nil)
	_ driver.RowsColumnTypeScanType         = (*Rows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*Rows)(nil)
	_ driver.RowsColumnTypeNullable         = (*Rows)(nil)
	_ driver.RowsColumnTypeLength           = (*Rows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*Rows)(nil)
)
