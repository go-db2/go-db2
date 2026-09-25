package network

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// The SQLDARD a Db2 12.1 server sent for
// SELECT 1 AS "abc", 2 AS "zamówienia" FROM SYSIBM.SYSDUMMY1.
// Column names arrive as UTF-8; one outside ASCII was replaced by PARAMn.
const sqldardUTF8Names = "0000000000303030303053514c3132303135000000000000000000010000000100000021ffffff00000000" +
	"2020202020202020202020001254455354444220202020202020202020202000000000ffff0200000000000400000000000000" +
	"f001000000000000000000000000000000000361626300000000000000000000ffffff000000000400000000000000f0010000" +
	"00000000000000000000000000000b7a616dc3b37769656e696100000000000000000000ffffff"

func TestParseSQLDARD_UTF8ColumnNames(t *testing.T) {
	obj, err := hex.DecodeString(sqldardUTF8Names)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := ParseSQLDARD(obj, binary.LittleEndian)
	if err != nil {
		t.Fatalf("ParseSQLDARD failed: %v", err)
	}
	var names []string
	for _, c := range cols {
		names = append(names, c.Name)
	}
	if len(names) != 2 || names[0] != "abc" || names[1] != "zamówienia" {
		t.Fatalf("column names = %q, want [abc zamówienia]", names)
	}
}

func BenchmarkParseSQLDARD(b *testing.B) {
	obj, err := hex.DecodeString(sqldardUTF8Names)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseSQLDARD(obj, binary.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}
