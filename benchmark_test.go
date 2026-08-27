package db2

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-db2/go-db2/converters"
	"github.com/go-db2/go-db2/types"
)

func getBenchmarkDSN() string {
	dsn := os.Getenv("DB2_DSN")
	if dsn == "" {
		dsn = "db2://db2inst1:MinhaSenhaForte123@127.0.0.1:50000/TESTDB?ssl=false"
	}
	return dsn
}

func BenchmarkDecodeField_Integer(b *testing.B) {
	data := []byte{0x00, 0x00, 0x04, 0xD2} // 1234
	ps := []byte{0x00, 0x04}
	endian := binary.BigEndian

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, err := converters.DecodeField(converters.DRDATypeInteger, ps, r, endian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeField_VarChar(b *testing.B) {
	text := "IBM Db2 Pure Go Driver High Performance"
	strBytes := []byte(text)
	data := make([]byte, 2+len(strBytes))
	binary.BigEndian.PutUint16(data[0:2], uint16(len(strBytes)))
	copy(data[2:], strBytes)
	ps := []byte{0x00, 0x50}
	endian := binary.BigEndian

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, err := converters.DecodeField(converters.DRDATypeVarChar, ps, r, endian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeField_PackedDecimal(b *testing.B) {
	// 123456.78 in packed decimal (precision 8, scale 2 -> 5 bytes: 0x01, 0x23, 0x45, 0x67, 0x8C)
	data := []byte{0x01, 0x23, 0x45, 0x67, 0x8C}
	ps := []byte{0x08, 0x02}
	endian := binary.BigEndian

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, err := converters.DecodeField(converters.DRDATypeDecimal, ps, r, endian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildSQLDTA(b *testing.B) {
	colTypes := []types.SQLType{types.SQLTypeInteger, types.SQLTypeVarChar, types.SQLTypeDecimal}
	colLens := []int64{4, 100, 10}
	precs := []int{0, 0, 10}
	scales := []int{0, 0, 2}
	args := []any{42, "Benchmark Product", 199.99}
	endian := binary.BigEndian

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := converters.BuildSQLDTA(colTypes, colLens, precs, scales, args, endian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLive_Ping(b *testing.B) {
	db, err := sql.Open("db2", getBenchmarkDSN())
	if err != nil {
		b.Skipf("Db2 not available: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		b.Skipf("Db2 ping failed, skipping live benchmark: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.Ping(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLive_SimpleQuery(b *testing.B) {
	db, err := sql.Open("db2", getBenchmarkDSN())
	if err != nil {
		b.Skipf("Db2 not available: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		b.Skipf("Db2 ping failed, skipping live benchmark: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var val int
		err := db.QueryRow("SELECT 1 FROM SYSIBM.SYSDUMMY1").Scan(&val)
		if err != nil {
			b.Fatal(err)
		}
		if val != 1 {
			b.Fatalf("expected 1, got %d", val)
		}
	}
}

func BenchmarkLive_PreparedQuery(b *testing.B) {
	db, err := sql.Open("db2", getBenchmarkDSN())
	if err != nil {
		b.Skipf("Db2 not available: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		b.Skipf("Db2 ping failed, skipping live benchmark: %v", err)
	}

	_, _ = db.Exec("DROP TABLE bench_test")
	_, err = db.Exec("CREATE TABLE bench_test (id INT NOT NULL, name VARCHAR(50))")
	if err != nil {
		b.Fatalf("failed to create bench_test: %v", err)
	}
	defer func() {
		_, _ = db.Exec("DROP TABLE bench_test")
	}()

	for i := 1; i <= 10; i++ {
		_, err := db.Exec(fmt.Sprintf("INSERT INTO bench_test VALUES (%d, 'Name %d')", i, i))
		if err != nil {
			b.Fatalf("failed to seed bench_test: %v", err)
		}
	}

	stmt, err := db.Prepare("SELECT name FROM bench_test WHERE id = ?")
	if err != nil {
		b.Fatalf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := (i % 10) + 1
		var name string
		err := stmt.QueryRow(id).Scan(&name)
		if err != nil {
			b.Fatal(err)
		}
	}
}
