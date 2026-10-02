package network

import (
	"bytes"
	"database/sql/driver"
	"encoding/binary"
	"testing"

	"github.com/go-db2/go-db2/converters"
)

// Db2 describes an XML result column as FD:OCA type 0xC7 (0xC6 when not
// nullable) with an 8-byte-length placeholder in the row, and sends the
// serialized value as EXTDTA, the way it sends LOBs. These are the bytes a
// Db2 12.1 server sent for SELECT ID, C_XML FROM a one-row table.
func TestXMLColumnIsStitchedFromEXTDTA(t *testing.T) {
	fields := []FieldDescriptor{
		{Type: converters.DRDATypeInteger, PS: [2]byte{0x00, 0x04}},
		{Type: 0xC7, PS: [2]byte{0x80, 0x09}},
	}
	row := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	extdta := [][]byte{append([]byte{0x00}, `<root><item id="1">x</item></root>`...)}

	r := bytes.NewReader(row)
	decoded := make([]driver.Value, len(fields))
	for i, f := range fields {
		v, err := converters.DecodeField(f.Type, f.PS[:], r, binary.LittleEndian)
		if err != nil {
			t.Fatalf("decoding column %d: %v", i, err)
		}
		decoded[i] = v
	}
	if r.Len() != 0 {
		t.Fatalf("%d bytes of the row were left unread", r.Len())
	}

	rows := [][]driver.Value{decoded}
	stitchEXTDTA(fields, rows, extdta)

	if got, want := rows[0][1], `<root><item id="1">x</item></root>`; got != want {
		t.Fatalf("XML column = %#v, want %q", got, want)
	}
}

func TestNullXMLColumn(t *testing.T) {
	v, err := converters.DecodeField(0xC7, []byte{0x80, 0x09}, bytes.NewReader([]byte{0xFF}), binary.LittleEndian)
	if err != nil {
		t.Fatalf("decoding a NULL XML value: %v", err)
	}
	if v != nil {
		t.Fatalf("NULL XML value decoded as %#v, want nil", v)
	}
}
