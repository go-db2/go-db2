package network

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/go-db2/go-db2/converters"
)

// rowSQLCA is the SQLCA a row starts with when the server attaches a warning
// or error to it, instead of the null indicator 0xFF.
func rowSQLCA(code int32, state string) []byte {
	b := []byte{0x00}
	b = binary.LittleEndian.AppendUint32(b, uint32(code))
	b = append(b, state...)
	b = append(b, "SQLRI6B1"...)
	b = append(b, 0x00)                // SQLCAXGRP present
	b = append(b, make([]byte, 24)...) // SQLERRD1-6
	b = append(b, "W W        "...)    // SQLWARN0-A
	b = append(b, 0x00, 0x12)          // SQLRDBNAME
	b = append(b, "TESTDB            "...)
	b = append(b, 0x00, 0x00, 0x00, 0x00) // SQLERRMSG_m, SQLERRMSG_s
	b = append(b, 0xFF)                   // no SQLDIAGGRP
	return b
}

// A row the server attaches a warning to (here SQLSTATE 01003, null values
// eliminated from an aggregate, which Db2 12.1 sent for an AVG/STDDEV row)
// starts with a full SQLCA instead of 0xFF, and must still be decoded.
func TestDecodeRowsWithWarningSQLCA(t *testing.T) {
	fields := []FieldDescriptor{{Type: converters.DRDATypeInteger, PS: []byte{0x00, 0x04}}}

	var data []byte
	data = append(data, rowSQLCA(0, "01003")...)
	data = append(data, 0x00) // row data present
	data = binary.LittleEndian.AppendUint32(data, 40)
	data = append(data, integerRows(41)...) // a plain row after it

	rows, err := decodeRows(fields, data, binary.LittleEndian)
	if err != nil {
		t.Fatalf("decodeRows failed: %v", err)
	}
	if len(rows) != 2 || rows[0][0] != int64(40) || rows[1][0] != int64(41) {
		t.Fatalf("rows = %v, want [[40] [41]]", rows)
	}
}

// A row the server attaches an error to must fail the query, not end it.
func TestDecodeRowsWithErrorSQLCA(t *testing.T) {
	fields := []FieldDescriptor{{Type: converters.DRDATypeInteger, PS: []byte{0x00, 0x04}}}

	var data []byte
	data = append(data, integerRows(1)...)
	data = append(data, rowSQLCA(-802, "22003")...)
	data = append(data, 0xFF) // no row data

	_, err := decodeRows(fields, data, binary.LittleEndian)
	if err == nil || !strings.Contains(err.Error(), "SQLCODE=-802") {
		t.Fatalf("expected the row's SQLCODE -802, got: %v", err)
	}
}

func BenchmarkDecodeRows(b *testing.B) {
	fields := []FieldDescriptor{
		{Type: converters.DRDATypeInteger, PS: []byte{0x00, 0x04}},
		{Type: converters.DRDATypeVarChar, PS: []byte{0x00, 0x20}},
	}
	// Create payload with 100 rows
	var data []byte
	for i := 0; i < 100; i++ {
		data = append(data, 0xFF, 0x00) // null SQLCA, row data present
		data = binary.LittleEndian.AppendUint32(data, uint32(i))
		str := "Row Value 12"
		data = binary.BigEndian.AppendUint16(data, uint16(len(str)))
		data = append(data, str...)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decodeRows(fields, data, binary.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}
