package codec

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildSQLDTA(t *testing.T) {
	colTypes := []SQLType{
		SQLTypeNInteger,
		SQLTypeNVarChar,
		SQLTypeNFloat,
		SQLTypeNBoolean,
		SQLTypeNDate,
	}
	colLens := []int64{4, 100, 8, 2, 10}
	precs := []int{0, 0, 0, 0, 0}
	scales := []int{0, 0, 0, 0, 0}

	d, _ := time.Parse("2006-01-02", "2026-08-26")
	args := []any{1, "Teclado Mecânico", 250.50, true, d}

	sqldta, err := BuildSQLDTA(colTypes, colLens, precs, scales, args, binary.LittleEndian)
	if err != nil {
		t.Fatalf("BuildSQLDTA failed: %v", err)
	}

	gotHex := hex.EncodeToString(sqldta)
	expectedHex := "00642412001c00101276d0030004393fff0b000805000221000a0671e4d000010044147a000001000000000010005400650063006c00610064006f0020004d0065006300e2006e00690063006f000000000000506f4000010000323032362d30382d3236"

	if gotHex != expectedHex {
		t.Errorf("BuildSQLDTA output mismatch!\nGot:      %s\nExpected: %s", gotHex, expectedHex)
	}
}

func TestEncodePackedDecimalParam_InvalidPrecScaleBounds(t *testing.T) {
	testCases := []struct {
		name  string
		prec  int
		scale int
	}{
		{"NegativeScale", 10, -1},
		{"NegativePrec", -5, 0},
		{"ZeroPrec", 0, 0},
		{"PrecExceedsMax", 32, 2},
		{"ScaleExceedsPrec", 5, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FDODTA panicked on invalid prec/scale: %v", r)
				}
			}()

			_, err := FDODTA(SQLTypeDecimal, 0, tc.prec, tc.scale, "123.45", binary.BigEndian)
			if err == nil {
				t.Fatalf("expected error for invalid prec=%d, scale=%d; got nil", tc.prec, tc.scale)
			}
		})
	}
}

func TestToBool(t *testing.T) {
	testCases := []struct {
		name     string
		input    any
		expected bool
	}{
		{"Bool_True", true, true},
		{"Bool_False", false, false},
		{"Int_Positive", 1, true},
		{"Int_Negative", -1, true},
		{"Int_Zero", 0, false},
		{"Int8_Positive", int8(1), true},
		{"Int8_Zero", int8(0), false},
		{"Int16_Positive", int16(1), true},
		{"Int16_Zero", int16(0), false},
		{"Int32_Positive", int32(1), true},
		{"Int32_Zero", int32(0), false},
		{"Int64_Positive", int64(42), true},
		{"Int64_Negative", int64(-42), true},
		{"Int64_Zero", int64(0), false},
		{"Uint_Positive", uint(1), true},
		{"Uint_Zero", uint(0), false},
		{"Uint8_Positive", uint8(1), true},
		{"Uint8_Zero", uint8(0), false},
		{"Uint16_Positive", uint16(1), true},
		{"Uint16_Zero", uint16(0), false},
		{"Uint32_Positive", uint32(1), true},
		{"Uint32_Zero", uint32(0), false},
		{"Uint64_Positive", uint64(1), true},
		{"Uint64_Zero", uint64(0), false},
		{"Float32_Positive", float32(1.5), true},
		{"Float32_Zero", float32(0.0), false},
		{"Float64_Positive", 1.0, true},
		{"Float64_Zero", 0.0, false},
		{"String_True_Lowercase", "true", true},
		{"String_True_Uppercase", "TRUE", true},
		{"String_True_MixedCase", "True", true},
		{"String_One", "1", true},
		{"String_False", "false", false},
		{"String_Zero", "0", false},
		{"String_Random", "random", false},
		{"String_Empty", "", false},
		{"Default_Nil", nil, false},
		{"Default_ByteSlice", []byte{1}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := toBool(tc.input)
			if got != tc.expected {
				t.Errorf("toBool(%v) = %v; want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestEncodePackedDecimalParam_TableDriven(t *testing.T) {
	testCases := []struct {
		name        string
		val         any
		prec        int
		scale       int
		expectedHex string
	}{
		{
			name:        "Float64_Positive_EvenPrec",
			val:         199.99,
			prec:        10,
			scale:       2,
			expectedHex: "0000000019999c",
		},
		{
			name:        "Float64_Negative_EvenPrec",
			val:         -199.99,
			prec:        10,
			scale:       2,
			expectedHex: "0000000019999d",
		},
		{
			name:        "String_Positive_OddPrec",
			val:         "123.45",
			prec:        5,
			scale:       2,
			expectedHex: "0012345c",
		},
		{
			name:        "String_Negative_OddPrec",
			val:         "-123.45",
			prec:        5,
			scale:       2,
			expectedHex: "0012345d",
		},
		{
			name:        "Int_Zero_OddPrec",
			val:         0,
			prec:        5,
			scale:       2,
			expectedHex: "0000000c",
		},
		{
			name:        "String_FracOnly_OddPrec",
			val:         "0.5",
			prec:        3,
			scale:       2,
			expectedHex: "00050c",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := encodePackedDecimalParam(tc.val, tc.prec, tc.scale)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotHex := hex.EncodeToString(b)
			if gotHex != tc.expectedHex {
				t.Errorf("mismatch!\nGot:      %s\nExpected: %s", gotHex, tc.expectedHex)
			}
		})
	}
}

func TestBuildSQLDTAWithEXTDTA_LargeLOB(t *testing.T) {
	colTypes := []SQLType{SQLTypeBlob, SQLTypeClob}
	colLens := []int64{200000, 200000}
	precs := []int{0, 0}
	scales := []int{0, 0}

	payloadBlob := make([]byte, 100000)
	for i := range payloadBlob {
		payloadBlob[i] = byte(i % 256)
	}
	payloadClob := strings.Repeat("A", 80000)

	sqldta, extdta, err := BuildSQLDTAWithEXTDTA(colTypes, colLens, precs, scales, []any{payloadBlob, payloadClob}, binary.BigEndian)
	if err != nil {
		t.Fatalf("BuildSQLDTAWithEXTDTA failed: %v", err)
	}

	if len(extdta) != 2 {
		t.Fatalf("expected 2 EXTDTA packets, got %d", len(extdta))
	}

	// Verify EXTDTA packet 1 (BLOB): 0x8008 extended length and CodePointEXTDTA (0x146C)
	pkt1 := extdta[0]
	if binary.BigEndian.Uint16(pkt1[0:2]) != 0x8008 {
		t.Errorf("expected extended DDM length 0x8008, got 0x%04X", binary.BigEndian.Uint16(pkt1[0:2]))
	}
	if binary.BigEndian.Uint16(pkt1[2:4]) != 0x146C {
		t.Errorf("expected EXTDTA codepoint 0x146C, got 0x%04X", binary.BigEndian.Uint16(pkt1[2:4]))
	}
	if binary.BigEndian.Uint32(pkt1[4:8]) != uint32(1+len(payloadBlob)) {
		t.Errorf("expected payload length %d, got %d", 1+len(payloadBlob), binary.BigEndian.Uint32(pkt1[4:8]))
	}

	// Verify EXTDTA packet 2 (CLOB)
	pkt2 := extdta[1]
	if binary.BigEndian.Uint16(pkt2[0:2]) != 0x8008 {
		t.Errorf("expected extended DDM length 0x8008, got 0x%04X", binary.BigEndian.Uint16(pkt2[0:2]))
	}
	if binary.BigEndian.Uint16(pkt2[2:4]) != 0x146C {
		t.Errorf("expected EXTDTA codepoint 0x146C, got 0x%04X", binary.BigEndian.Uint16(pkt2[2:4]))
	}
	if binary.BigEndian.Uint32(pkt2[4:8]) != uint32(1+len(payloadClob)) {
		t.Errorf("expected payload length %d, got %d", 1+len(payloadClob), binary.BigEndian.Uint32(pkt2[4:8]))
	}

	// Verify FDODSC has LOB descriptors
	if !bytes.Contains(sqldta, []byte{0xC9, 0x80, 0x04}) {
		t.Errorf("expected FDODSC to contain BLOB descriptor {0xC9, 0x80, 0x04}")
	}
	if !bytes.Contains(sqldta, []byte{0xCF, 0x80, 0x04}) {
		t.Errorf("expected FDODSC to contain CLOB descriptor {0xCF, 0x80, 0x04}")
	}
}

func TestScalarInlineOverflow(t *testing.T) {
	colTypes := []SQLType{SQLTypeVarChar}
	colLens := []int64{100000}
	precs := []int{0}
	scales := []int{0}

	bigString := strings.Repeat("A", 70000)
	_, _, err := BuildSQLDTAWithEXTDTA(colTypes, colLens, precs, scales, []any{bigString}, binary.BigEndian)
	if err == nil {
		t.Fatalf("expected overflow error for VARCHAR > 65535, got nil")
	}
}

// AC-3: LOB Stream DoS Guard
func TestBuildSQLDTAWithEXTDTA_LOBStreamExceedsLimit_AC3(t *testing.T) {
	oldLimit := MaxLOBStreamSize
	MaxLOBStreamSize = 1024
	defer func() {
		MaxLOBStreamSize = oldLimit
	}()

	colTypes := []SQLType{SQLTypeBlob}
	colLens := []int64{2048}
	precs := []int{0}
	scales := []int{0}

	oversizedStream := bytes.NewReader(make([]byte, 1025))

	_, _, err := BuildSQLDTAWithEXTDTA(colTypes, colLens, precs, scales, []any{oversizedStream}, binary.BigEndian)
	if err == nil {
		t.Fatalf("expected error when stream exceeds MaxLOBStreamSize, got nil")
	}
	if !errors.Is(err, ErrLOBStreamTooLarge) {
		t.Fatalf("expected ErrLOBStreamTooLarge, got: %v", err)
	}
	expectedMsg := "parameter 1 exceeds maximum allowed size of 1024 bytes"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error message containing %q, got %q", expectedMsg, err.Error())
	}
}

// AC-4: LOB Stream Sem Truncamento
func TestBuildSQLDTAWithEXTDTA_ValidLOBStreamNoTruncation_AC4(t *testing.T) {
	colTypes := []SQLType{SQLTypeBlob}
	colLens := []int64{100000}
	precs := []int{0}
	scales := []int{0}

	streamSize := 65536 // 64 KiB
	payload := make([]byte, streamSize)
	for i := 0; i < streamSize; i++ {
		payload[i] = byte(i % 256)
	}

	stream := bytes.NewReader(payload)
	_, extdta, err := BuildSQLDTAWithEXTDTA(colTypes, colLens, precs, scales, []any{stream}, binary.BigEndian)
	if err != nil {
		t.Fatalf("BuildSQLDTAWithEXTDTA failed: %v", err)
	}

	if len(extdta) != 1 {
		t.Fatalf("expected 1 EXTDTA packet, got %d", len(extdta))
	}

	pkt := extdta[0]
	if binary.BigEndian.Uint16(pkt[0:2]) != 0x8008 {
		t.Errorf("expected extended DDM length 0x8008, got 0x%04X", binary.BigEndian.Uint16(pkt[0:2]))
	}
	if binary.BigEndian.Uint16(pkt[2:4]) != 0x146C {
		t.Errorf("expected EXTDTA codepoint 0x146C, got 0x%04X", binary.BigEndian.Uint16(pkt[2:4]))
	}
	if binary.BigEndian.Uint32(pkt[4:8]) != uint32(1+streamSize) {
		t.Errorf("expected payload length %d, got %d", 1+streamSize, binary.BigEndian.Uint32(pkt[4:8]))
	}
	if !bytes.Equal(pkt[9:], payload) {
		t.Fatalf("payload corrupted or truncated")
	}
}
