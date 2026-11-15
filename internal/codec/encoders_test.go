package codec

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
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

func TestToBoolStrict(t *testing.T) {
	testCases := []struct {
		name      string
		input     any
		expected  bool
		expectErr bool
	}{
		{"Bool_True", true, true, false},
		{"Bool_False", false, false, false},
		{"Int_One", 1, true, false},
		{"Int_Zero", 0, false, false},
		{"Int_NegativeOne", -1, false, true},
		{"Int_OutOfRange", 42, false, true},
		{"Int8_One", int8(1), true, false},
		{"Int8_Zero", int8(0), false, false},
		{"Int16_One", int16(1), true, false},
		{"Int16_Zero", int16(0), false, false},
		{"Int32_One", int32(1), true, false},
		{"Int32_Zero", int32(0), false, false},
		{"Int64_One", int64(1), true, false},
		{"Int64_Zero", int64(0), false, false},
		{"Int64_OutOfRange", int64(42), false, true},
		{"Uint_One", uint(1), true, false},
		{"Uint_Zero", uint(0), false, false},
		{"Uint8_One", uint8(1), true, false},
		{"Uint8_Zero", uint8(0), false, false},
		{"Uint16_One", uint16(1), true, false},
		{"Uint16_Zero", uint16(0), false, false},
		{"Uint32_One", uint32(1), true, false},
		{"Uint32_Zero", uint32(0), false, false},
		{"Uint64_One", uint64(1), true, false},
		{"Uint64_Zero", uint64(0), false, false},
		{"Uint64_OutOfRange", uint64(2), false, true},
		{"Float32_Invalid", float32(1.5), false, true},
		{"Float64_Zero_Invalid", 0.0, false, true},
		{"String_True_Lowercase", "true", true, false},
		{"String_True_Uppercase", "TRUE", true, false},
		{"String_True_MixedCase", "True", true, false},
		{"String_T", "t", true, false},
		{"String_One", "1", true, false},
		{"String_False", "false", false, false},
		{"String_F", "F", false, false},
		{"String_Zero", "0", false, false},
		{"String_Trimmed", " true ", true, false},
		{"String_Random", "random", false, true},
		{"String_Empty", "", false, true},
		{"String_Yes", "yes", false, true},
		{"Default_Nil", nil, false, true},
		{"Default_ByteSlice", []byte{1}, false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toBoolStrict(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("toBoolStrict(%v) expected error, got value %v", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("toBoolStrict(%v) unexpected error: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("toBoolStrict(%v) = %v; want %v", tc.input, got, tc.expected)
			}
		})
	}

	if _, err := FDODTA(SQLTypeBoolean, 1, 0, 0, "random", binary.BigEndian); err == nil {
		t.Fatal("expected FDODTA(SQLTypeBoolean, \"random\") to return error")
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

func TestFDODTA_StrictValidation_SEC05(t *testing.T) {
	t.Run("GRAPHIC strict validation", func(t *testing.T) {
		// Valid string and UTF-8 []byte within capacity
		if _, err := FDODTA(SQLTypeGraphic, 5, 0, 0, "ABC", binary.BigEndian); err != nil {
			t.Fatalf("unexpected error for valid GRAPHIC string: %v", err)
		}
		if _, err := FDODTA(SQLTypeGraphic, 5, 0, 0, []byte("日本"), binary.BigEndian); err != nil {
			t.Fatalf("unexpected error for valid GRAPHIC UTF-8 []byte: %v", err)
		}

		// Silent truncation previously occurred for > sqllen
		if _, err := FDODTA(SQLTypeGraphic, 5, 0, 0, "ABCDEFGHIJ", binary.BigEndian); err == nil {
			t.Fatal("expected error when GRAPHIC length exceeds sqllen")
		}
		// Non-string/[]byte types must fail
		if _, err := FDODTA(SQLTypeGraphic, 10, 0, 0, 123, binary.BigEndian); err == nil {
			t.Fatal("expected error for int in GRAPHIC")
		}
		// Invalid UTF-8
		if _, err := FDODTA(SQLTypeGraphic, 10, 0, 0, []byte{0xff, 0xfe}, binary.BigEndian); err == nil {
			t.Fatal("expected error for invalid UTF-8 bytes in GRAPHIC")
		}
		if _, err := FDODTA(SQLTypeGraphic, 10, 0, 0, string([]byte{0xff}), binary.BigEndian); err == nil {
			t.Fatal("expected error for invalid UTF-8 string in GRAPHIC")
		}
		// Input string length exceeding uint16 before UTF-16 encoding
		if _, err := FDODTA(SQLTypeGraphic, 0x7FFF, 0, 0, strings.Repeat("A", 65536), binary.BigEndian); err == nil {
			t.Fatal("expected error when GRAPHIC input string length exceeds 65535")
		}
		// Out-of-bounds sqllen
		for _, badLen := range []int64{0, -1, 0x8000} {
			if _, err := FDODTA(SQLTypeGraphic, badLen, 0, 0, "A", binary.BigEndian); err == nil {
				t.Fatalf("expected error for GRAPHIC sqllen=%d", badLen)
			}
			if _, _, err := BuildSQLDTAWithEXTDTA([]SQLType{SQLTypeGraphic}, []int64{badLen}, []int{0}, []int{0}, []any{nil}, binary.BigEndian); err == nil {
				t.Fatalf("expected FDODSC error for GRAPHIC sqllen=%d even with nil arg", badLen)
			}
		}
	})

	t.Run("XML and default inline length overflow", func(t *testing.T) {
		if _, err := FDODTA(SQLTypeXML, 0, 0, 0, "<root>ok</root>", binary.BigEndian); err != nil {
			t.Fatalf("unexpected error for valid XML: %v", err)
		}
		if _, err := FDODTA(SQLTypeXML, 0, 0, 0, 123, binary.BigEndian); err == nil {
			t.Fatal("expected error for int in XML")
		}
		if _, err := FDODTA(SQLTypeXML, 0, 0, 0, []byte{0xff}, binary.BigEndian); err == nil {
			t.Fatal("expected error for invalid UTF-8 in XML")
		}
		// Raw string length > 65535
		if _, err := FDODTA(SQLTypeXML, 0, 0, 0, strings.Repeat("A", 65536), binary.BigEndian); err == nil {
			t.Fatal("expected error for XML input length > 65535")
		}
		if _, err := FDODTA(SQLType(9999), 0, 0, 0, strings.Repeat("A", 65536), binary.BigEndian); err == nil {
			t.Fatal("expected error for default fallback input length > 65535")
		}
		// 40,000 ASCII chars -> 80,000 UTF-16BE bytes (> 65535)
		if _, err := FDODTA(SQLTypeXML, 0, 0, 0, strings.Repeat("A", 40000), binary.BigEndian); err == nil {
			t.Fatal("expected error for XML UTF-16BE byte length > 65535")
		}
		if _, err := FDODTA(SQLType(9999), 0, 0, 0, strings.Repeat("A", 40000), binary.BigEndian); err == nil {
			t.Fatal("expected error for default fallback UTF-16BE byte length > 65535")
		}
	})

	t.Run("DECIMAL strict parsing and FDODSC bounds", func(t *testing.T) {
		// FDODSC validation even with nil argument
		badPrecScales := [][2]int{{0, 0}, {-1, 0}, {32, 2}, {5, 6}, {5, -1}}
		for _, ps := range badPrecScales {
			if _, _, err := BuildSQLDTAWithEXTDTA([]SQLType{SQLTypeDecimal}, []int64{0}, []int{ps[0]}, []int{ps[1]}, []any{nil}, binary.BigEndian); err == nil {
				t.Fatalf("expected BuildSQLDTAWithEXTDTA error for invalid DECIMAL(%d,%d) even with nil", ps[0], ps[1])
			}
		}

		// Valid canonical/trailing-zero/leading-zero inputs
		valid := []struct {
			val   any
			prec  int
			scale int
		}{
			{"00123.45", 5, 2},
			{"+123.45", 5, 2},
			{"123.450", 5, 2},
			{"-123.45", 5, 2},
			{123.45, 5, 2},
			{float32(12.5), 5, 2},
			{uint8(99), 5, 2},
		}
		for _, tc := range valid {
			if _, err := encodePackedDecimalParam(tc.val, tc.prec, tc.scale); err != nil {
				t.Fatalf("expected valid DECIMAL(%d,%d) for %v (%T), got err: %v", tc.prec, tc.scale, tc.val, tc.val, err)
			}
		}

		// Invalid values that MUST error (no silent truncation or zeroing)
		invalid := []struct {
			name  string
			val   any
			prec  int
			scale int
		}{
			{"NaN", math.NaN(), 10, 2},
			{"PosInf", math.Inf(1), 10, 2},
			{"NegInf", math.Inf(-1), 10, 2},
			{"FloatOverflow1e35", 1e35, 31, 2},
			{"FloatTiny1e-35", 1e-35, 31, 31},
			{"Bool", true, 5, 2},
			{"Bytes", []byte("12.34"), 5, 2},
			{"Struct", struct{}{}, 5, 2},
			{"DoubleDot", "1.2.3", 5, 2},
			{"AlphaDigits", "12abc.34", 5, 2},
			{"SciNotation", "1e5", 10, 0},
			{"DoubleMinus", "--5", 5, 2},
			{"SignOnlyPlus", "+", 5, 2},
			{"SignOnlyMinus", "-", 5, 2},
			{"DotOnly", ".", 5, 2},
			{"EmptyString", "", 5, 2},
			{"IntOverflowString", "12345.67", 5, 2},
			{"FracLossString", "123.456", 5, 2},
			{"FracLossFloat64", 123.456, 5, 2},
			{"IntOverflowFloat32", float32(123.5), 4, 2},
		}
		for _, tc := range invalid {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := encodePackedDecimalParam(tc.val, tc.prec, tc.scale); err == nil {
					t.Fatalf("expected error for %v (%T) on DECIMAL(%d,%d), got nil", tc.val, tc.val, tc.prec, tc.scale)
				}
			})
		}
	})

	t.Run("FLOAT strict validation and float32 overflow", func(t *testing.T) {
		for _, v := range []any{int(1), int8(-2), int16(3), int32(4), int64(5), uint(6), uint8(7), uint16(8), uint32(9), uint64(10), float32(1.5), float64(2.5)} {
			if _, err := FDODTA(SQLTypeFloat, 8, 0, 0, v, binary.BigEndian); err != nil {
				t.Fatalf("unexpected error for %T in FLOAT(8): %v", v, err)
			}
		}
		for _, v := range []any{math.NaN(), math.Inf(1), math.Inf(-1), "12.34", true, []byte{1}, struct{}{}} {
			if _, err := FDODTA(SQLTypeFloat, 8, 0, 0, v, binary.BigEndian); err == nil {
				t.Fatalf("expected error for %v (%T) in FLOAT(8)", v, v)
			}
		}
		if _, err := FDODTA(SQLTypeFloat, 4, 0, 0, math.MaxFloat64, binary.BigEndian); err == nil {
			t.Fatal("expected REAL (sqllen=4) overflow error for math.MaxFloat64")
		}
		if _, err := FDODTA(SQLTypeFloat, 8, 0, 0, math.MaxFloat64, binary.BigEndian); err != nil {
			t.Fatalf("unexpected error for DOUBLE (sqllen=8) with math.MaxFloat64: %v", err)
		}
	})

	t.Run("BINARY strict validation", func(t *testing.T) {
		for _, bad := range []any{123, struct{}{}} {
			if _, err := FDODTA(SQLTypeBinary, 4, 0, 0, bad, binary.BigEndian); err == nil {
				t.Fatalf("expected error for %T in BINARY", bad)
			}
			if _, err := FDODTA(SQLTypeVarBinary, 4, 0, 0, bad, binary.BigEndian); err == nil {
				t.Fatalf("expected error for %T in VARBINARY", bad)
			}
			if _, _, err := BuildSQLDTAWithEXTDTA([]SQLType{SQLTypeBlob}, []int64{100}, []int{0}, []int{0}, []any{bad}, binary.BigEndian); err == nil {
				t.Fatalf("expected EXTDTA error for %T in BLOB", bad)
			}
		}
		// DBCLOB externalized string with invalid UTF-8 MUST fail
		invalidDBClob := strings.Repeat(string([]byte{0xff}), 16384)
		if _, _, err := BuildSQLDTAWithEXTDTA([]SQLType{SQLTypeDbClob}, []int64{100}, []int{0}, []int{0}, []any{invalidDBClob}, binary.BigEndian); err == nil {
			t.Fatal("expected EXTDTA error for invalid UTF-8 in DBCLOB")
		}
		if _, err := FDODTA(SQLTypeBinary, 2, 0, 0, []byte{1, 2, 3, 4}, binary.BigEndian); err == nil {
			t.Fatal("expected error when binary length exceeds BINARY(2) capacity")
		}
		for _, badLen := range []int64{0, -1, 0x8000} {
			if _, err := FDODTA(SQLTypeBinary, badLen, 0, 0, []byte{1}, binary.BigEndian); err == nil {
				t.Fatalf("expected error for BINARY sqllen=%d", badLen)
			}
			if _, _, err := BuildSQLDTAWithEXTDTA([]SQLType{SQLTypeBinary}, []int64{badLen}, []int{0}, []int{0}, []any{nil}, binary.BigEndian); err == nil {
				t.Fatalf("expected FDODSC error for BINARY sqllen=%d even with nil arg", badLen)
			}
		}
	})
}

func BenchmarkAppendPackedDecimalParam(b *testing.B) {
	cases := []struct {
		name  string
		val   any
		prec  int
		scale int
	}{
		{"Int64", int64(12345678), 10, 2},
		{"Float64", 199.99, 10, 2},
		{"String", "00123.450", 5, 2},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			dst := make([]byte, 0, 32)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				dst, err = appendPackedDecimalParam(dst[:0], tc.val, tc.prec, tc.scale)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkToFloat64Strict(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := toFloat64Strict(123.45); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToBoolStrict(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := toBoolStrict("true"); err != nil {
			b.Fatal(err)
		}
	}
}
