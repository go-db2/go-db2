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

func TestDecodeFieldBooleanZeroLength(t *testing.T) {
	// Zero-length boolean parameter metadata should not panic
	var buf bytes.Buffer
	v, err := DecodeField(DRDATypeBoolean, []byte{0x00, 0x00}, &buf, binary.LittleEndian)
	if err != nil || v != false {
		t.Errorf("Boolean decode zero-length = %v, err: %v; want false", v, err)
	}

	buf.Reset()
	buf.WriteByte(0x00) // Null indicator = not null
	v, err = DecodeField(DRDATypeNBoolean, []byte{0x00, 0x00}, &buf, binary.LittleEndian)
	if err != nil || v != false {
		t.Errorf("Nullable Boolean decode zero-length = %v, err: %v; want false", v, err)
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

func TestParseSQLDTARD_TableDriven(t *testing.T) {
	// Helper to create an object header (uint16 len + uint16 codepoint)
	makeObj := func(cp uint16, body []byte) []byte {
		buf := make([]byte, 4+len(body))
		binary.BigEndian.PutUint16(buf[0:2], uint16(len(buf)))
		binary.BigEndian.PutUint16(buf[2:4], cp)
		copy(buf[4:], body)
		return buf
	}

	tests := []struct {
		name          string
		dscBytes      []byte
		dtaBytes      []byte
		expectedCount int
	}{
		{
			name: "SingleGroup_TwoFields",
			// Group 1: len=9 (header 3 + 2 fields * 3), 0x76 header
			// Field 1: Smallint (0x04, ps 0x00,0x02)
			// Field 2: Integer (0x02, ps 0x00,0x04)
			dscBytes: []byte{
				0x09, 0x76, 0xD0,
				DRDATypeSmall, 0x00, 0x02,
				DRDATypeInteger, 0x00, 0x04,
			},
			dtaBytes: func() []byte {
				var buf bytes.Buffer
				buf.Write([]byte{0xFF, 0x00}) // row header
				binary.Write(&buf, binary.BigEndian, int16(42))
				binary.Write(&buf, binary.BigEndian, int32(100))
				return buf.Bytes()
			}(),
			expectedCount: 2,
		},
		{
			name: "MultiGroup_ThreeFields",
			// Group 1: len=9, 2 fields
			// Group 2: len=6, 1 field (Smallint 0x04)
			dscBytes: []byte{
				0x09, 0x76, 0xD0,
				DRDATypeSmall, 0x00, 0x02,
				DRDATypeInteger, 0x00, 0x04,
				0x06, 0x76, 0xD0,
				DRDATypeSmall, 0x00, 0x02,
			},
			dtaBytes: func() []byte {
				var buf bytes.Buffer
				buf.Write([]byte{0xFF, 0x00}) // row header
				binary.Write(&buf, binary.BigEndian, int16(10))
				binary.Write(&buf, binary.BigEndian, int32(20))
				binary.Write(&buf, binary.BigEndian, int16(30))
				return buf.Bytes()
			}(),
			expectedCount: 3,
		},
		{
			name: "TruncatedGroupHeader",
			// Group len claims 20 bytes but payload is only 5 bytes
			dscBytes: []byte{
				0x14, 0x76, 0xD0, 0x04, 0x00,
			},
			dtaBytes:      []byte{0xFF, 0x00, 0x00, 0x01},
			expectedCount: 0,
		},
		{
			name:          "EmptyFDODTA",
			dscBytes:      []byte{0x06, 0x76, 0xD0, DRDATypeSmall, 0x00, 0x02},
			dtaBytes:      nil,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload []byte
			if len(tt.dscBytes) > 0 {
				payload = append(payload, makeObj(0x0010, tt.dscBytes)...) // FDODSC
			}
			if len(tt.dtaBytes) > 0 {
				payload = append(payload, makeObj(0x147A, tt.dtaBytes)...) // FDODTA
			}

			res, err := ParseSQLDTARD(payload, binary.BigEndian)
			if err != nil {
				t.Fatalf("ParseSQLDTARD failed: %v", err)
			}
			if len(res) != tt.expectedCount {
				t.Fatalf("expected %d decoded fields, got %d", tt.expectedCount, len(res))
			}
		})
	}
}
