package converters

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"github.com/go-db2/go-db2/types"
)

func TestBuildSQLDTA(t *testing.T) {
	colTypes := []types.SQLType{
		types.SQLTypeNInteger,
		types.SQLTypeNVarChar,
		types.SQLTypeNFloat,
		types.SQLTypeNBoolean,
		types.SQLTypeNDate,
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

			_, err := FDODTA(types.SQLTypeDecimal, 0, tc.prec, tc.scale, "123.45", binary.BigEndian)
			if err == nil {
				t.Fatalf("expected error for invalid prec=%d, scale=%d; got nil", tc.prec, tc.scale)
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
