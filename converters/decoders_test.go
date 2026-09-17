package converters

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestDecodeFieldIntegers(t *testing.T) {
	// Smallint: 42
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, int16(42))
	v, err := DecodeField(DRDATypeSmall, []byte{0x00, 0x02}, &buf, binary.LittleEndian)
	if err != nil || v.(int64) != 42 {
		t.Errorf("Smallint decode = %v, err: %v; want 42", v, err)
	}

	// Nullable Integer with Null indicator (0xFF)
	buf.Reset()
	buf.WriteByte(0xFF)
	v, err = DecodeField(DRDATypeNInteger, []byte{0x00, 0x04}, &buf, binary.LittleEndian)
	if err != nil || v != nil {
		t.Errorf("Nullable Integer null decode = %v, want nil", v)
	}

	// Nullable Integer with Value (0x00 + 1000)
	buf.Reset()
	buf.WriteByte(0x00)
	binary.Write(&buf, binary.LittleEndian, int32(1000))
	v, err = DecodeField(DRDATypeNInteger, []byte{0x00, 0x04}, &buf, binary.LittleEndian)
	if err != nil || v.(int64) != 1000 {
		t.Errorf("Nullable Integer decode = %v, want 1000", v)
	}
}

func TestDecodeFieldStrings(t *testing.T) {
	// VarChar: 2 bytes length (5) + "Hello"
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x05})
	buf.WriteString("Hello")
	v, err := DecodeField(DRDATypeVarChar, []byte{0x00, 0x0A}, &buf, binary.LittleEndian)
	if err != nil || v.(string) != "Hello" {
		t.Errorf("VarChar decode = %v, want 'Hello'", v)
	}

	// Char(10): "test      " -> trimmed "test"
	buf.Reset()
	buf.WriteString("test      ")
	v, err = DecodeField(DRDATypeChar, []byte{0x00, 0x0A}, &buf, binary.LittleEndian)
	if err != nil || v.(string) != "test" {
		t.Errorf("Char decode = %v, want 'test'", v)
	}
}

func TestDecodeFieldDateAndTimestamp(t *testing.T) {
	// Date: "2026-08-26"
	var buf bytes.Buffer
	buf.WriteString("2026-08-26")
	v, err := DecodeField(DRDATypeDate, []byte{0x00, 0x0A}, &buf, binary.LittleEndian)
	if err != nil {
		t.Fatalf("Date decode err: %v", err)
	}
	expectedDate, _ := time.Parse("2006-01-02", "2026-08-26")
	if v.(time.Time) != expectedDate {
		t.Errorf("Date = %v, want %v", v, expectedDate)
	}
}

func TestDecodeField_BooleanZeroLengthPanicProtection(t *testing.T) {
	var buf bytes.Buffer
	// Zero length ps metadata: [0x00, 0x00]
	v, err := DecodeField(DRDATypeBoolean, []byte{0x00, 0x00}, &buf, binary.LittleEndian)
	if err != nil {
		t.Fatalf("expected no error for zero-length boolean metadata, got %v", err)
	}
	if v != false {
		t.Errorf("expected false for zero-length boolean, got %v", v)
	}
}

func TestDecodePackedDecimal_LargePayload(t *testing.T) {
	// 40-byte packed decimal payload (> 32 bytes)
	// Example: 79 digits of '1' plus positive sign 'C'
	b := make([]byte, 40)
	for i := 0; i < 39; i++ {
		b[i] = 0x11
	}
	b[39] = 0x1C // Last digit 1, sign C (positive)

	res := DecodePackedDecimal(b, 0)
	if len(res) != 79 {
		t.Errorf("expected 79 digit string, got length %d: %s", len(res), res)
	}
}
