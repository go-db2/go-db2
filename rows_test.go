package db2

import (
	"database/sql/driver"
	"io"
	"reflect"
	"testing"

	"github.com/go-db2/go-db2/network"
	"github.com/go-db2/go-db2/types"
)

func TestRowsSingleResultSet(t *testing.T) {
	cols := []network.ColumnDescription{
		{Name: "ID", SQLType: uint16(types.SQLTypeInteger), Length: 4},
		{Name: "NAME", SQLType: uint16(types.SQLTypeVarChar), Length: 50, Nullable: true},
	}
	data := [][]driver.Value{
		{int64(1), "Alice"},
		{int64(2), "Bob"},
	}

	rows := NewRows(cols, data)

	names := rows.Columns()
	if len(names) != 2 || names[0] != "ID" || names[1] != "NAME" {
		t.Fatalf("expected [ID NAME], got %v", names)
	}

	dest := make([]driver.Value, 2)
	if err := rows.Next(dest); err != nil {
		t.Fatalf("unexpected error on 1st Next: %v", err)
	}
	if dest[0] != int64(1) || dest[1] != "Alice" {
		t.Fatalf("expected [1 Alice], got %v", dest)
	}

	if err := rows.Next(dest); err != nil {
		t.Fatalf("unexpected error on 2nd Next: %v", err)
	}
	if dest[0] != int64(2) || dest[1] != "Bob" {
		t.Fatalf("expected [2 Bob], got %v", dest)
	}

	if err := rows.Next(dest); err != io.EOF {
		t.Fatalf("expected io.EOF on 3rd Next, got %v", err)
	}

	if rows.HasNextResultSet() {
		t.Fatalf("expected HasNextResultSet to be false for single result set")
	}

	if err := rows.NextResultSet(); err != io.EOF {
		t.Fatalf("expected io.EOF on NextResultSet, got %v", err)
	}
}

func TestRowsMultipleResultSets(t *testing.T) {
	set1Cols := []network.ColumnDescription{
		{Name: "ID", SQLType: uint16(types.SQLTypeInteger)},
		{Name: "NAME", SQLType: uint16(types.SQLTypeVarChar)},
	}
	set1Data := [][]driver.Value{
		{int64(1), "Alice"},
		{int64(2), "Bob"},
	}

	set2Cols := []network.ColumnDescription{
		{Name: "TOTAL_USERS", SQLType: uint16(types.SQLTypeBigInt)},
		{Name: "AVG_AGE", SQLType: uint16(types.SQLTypeFloat)},
	}
	set2Data := [][]driver.Value{
		{int64(2), float64(27.5)},
	}

	multiRows := NewMultiRows([]ResultSet{
		{Columns: set1Cols, Data: set1Data},
		{Columns: set2Cols, Data: set2Data},
	})

	// 1. First Result Set
	cols1 := multiRows.Columns()
	if len(cols1) != 2 || cols1[0] != "ID" || cols1[1] != "NAME" {
		t.Fatalf("expected [ID NAME], got %v", cols1)
	}

	dest := make([]driver.Value, 2)
	if err := multiRows.Next(dest); err != nil {
		t.Fatalf("error on 1st Next: %v", err)
	}
	if dest[0] != int64(1) || dest[1] != "Alice" {
		t.Fatalf("expected [1 Alice], got %v", dest)
	}

	if err := multiRows.Next(dest); err != nil {
		t.Fatalf("error on 2nd Next: %v", err)
	}
	if dest[0] != int64(2) || dest[1] != "Bob" {
		t.Fatalf("expected [2 Bob], got %v", dest)
	}

	if err := multiRows.Next(dest); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Check NextResultSet availability
	if !multiRows.HasNextResultSet() {
		t.Fatalf("expected HasNextResultSet to be true")
	}

	// 2. Advance to Second Result Set
	if err := multiRows.NextResultSet(); err != nil {
		t.Fatalf("unexpected error on NextResultSet: %v", err)
	}

	cols2 := multiRows.Columns()
	if len(cols2) != 2 || cols2[0] != "TOTAL_USERS" || cols2[1] != "AVG_AGE" {
		t.Fatalf("expected [TOTAL_USERS AVG_AGE], got %v", cols2)
	}

	if err := multiRows.Next(dest); err != nil {
		t.Fatalf("error on 2nd set 1st Next: %v", err)
	}
	if dest[0] != int64(2) || dest[1] != float64(27.5) {
		t.Fatalf("expected [2 27.5], got %v", dest)
	}

	if err := multiRows.Next(dest); err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// No third set
	if multiRows.HasNextResultSet() {
		t.Fatalf("expected HasNextResultSet to be false after last set")
	}
	if err := multiRows.NextResultSet(); err != io.EOF {
		t.Fatalf("expected io.EOF on NextResultSet after last set, got %v", err)
	}
}

func TestRowsColumnTypes(t *testing.T) {
	cols := []network.ColumnDescription{
		{Name: "C_INT", SQLType: uint16(types.SQLTypeInteger), Length: 4, Nullable: false},
		{Name: "C_VARCHAR", SQLType: uint16(types.SQLTypeVarChar), Length: 100, Nullable: true},
		{Name: "C_DECIMAL", SQLType: uint16(types.SQLTypeDecimal), Precision: 10, Scale: 2, Nullable: true},
	}
	rows := NewRows(cols, nil)

	if rows.ColumnTypeScanType(0) != reflect.TypeOf(int32(0)) {
		t.Fatalf("expected int32 scan type, got %v", rows.ColumnTypeScanType(0))
	}
	if rows.ColumnTypeDatabaseTypeName(0) != "INTEGER" {
		t.Fatalf("expected INTEGER, got %s", rows.ColumnTypeDatabaseTypeName(0))
	}

	if nullable, ok := rows.ColumnTypeNullable(1); !ok || !nullable {
		t.Fatalf("expected nullable true for C_VARCHAR")
	}
	if ln, ok := rows.ColumnTypeLength(1); !ok || ln != 100 {
		t.Fatalf("expected length 100, got %d", ln)
	}

	if prec, scale, ok := rows.ColumnTypePrecisionScale(2); !ok || prec != 10 || scale != 2 {
		t.Fatalf("expected prec=10, scale=2, got prec=%d, scale=%d", prec, scale)
	}
}
