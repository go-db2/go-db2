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
