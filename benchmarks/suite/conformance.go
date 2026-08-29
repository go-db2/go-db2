package suite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

// RunConformanceSuite runs semantic equality checks comparing go-db2 and go_ibm_db.
func RunConformanceSuite(ctx context.Context, pgDB, cgoDB *sql.DB) []ConformanceResult {
	var results []ConformanceResult

	// 1. Basic Scalar Query
	results = append(results, compareQueryRow(ctx, "Basic Scalar (Int, String, Float)",
		pgDB, cgoDB,
		"SELECT 42, 'Pure Go Db2 Driver', 123.45 FROM SYSIBM.SYSDUMMY1",
		func(row *sql.Row) (string, error) {
			var i int
			var s string
			var f float64
			err := row.Scan(&i, &s, &f)
			return fmt.Sprintf("i=%d, s=%q, f=%.2f", i, s, f), err
		},
	))

	// 2. Null Values
	results = append(results, compareQueryRow(ctx, "Null Handling (sql.Null*)",
		pgDB, cgoDB,
		"SELECT CAST(NULL AS INTEGER), CAST(NULL AS VARCHAR(50)) FROM SYSIBM.SYSDUMMY1",
		func(row *sql.Row) (string, error) {
			var ni sql.NullInt64
			var ns sql.NullString
			err := row.Scan(&ni, &ns)
			return fmt.Sprintf("ni.Valid=%v, ns.Valid=%v", ni.Valid, ns.Valid), err
		},
	))

	// 3. Temporal Types (Current Date/Time)
	results = append(results, compareQueryRow(ctx, "Temporal (Current Date Format)",
		pgDB, cgoDB,
		"SELECT VARCHAR_FORMAT(CURRENT DATE, 'YYYY-MM-DD') FROM SYSIBM.SYSDUMMY1",
		func(row *sql.Row) (string, error) {
			var d string
			err := row.Scan(&d)
			return d, err
		},
	))

	// Setup comprehensive temporary test table
	_, _ = pgDB.ExecContext(ctx, "DROP TABLE BENCH_CONF_TYPES")
	createTableSQL := `
		CREATE TABLE BENCH_CONF_TYPES (
			ID INT NOT NULL PRIMARY KEY,
			C_SMALLINT SMALLINT,
			C_INT INTEGER,
			C_BIGINT BIGINT,
			C_REAL REAL,
			C_DOUBLE DOUBLE,
			C_DECIMAL DECIMAL(15,2),
			C_VARCHAR VARCHAR(100),
			C_DECFLOAT DECFLOAT(34),
			C_BLOB BLOB(64K),
			C_CLOB CLOB(64K),
			C_VARGRAPHIC VARGRAPHIC(100)
		)
	`
	if _, err := pgDB.ExecContext(ctx, createTableSQL); err != nil {
		results = append(results, ConformanceResult{
			Name:       "Setup Table BENCH_CONF_TYPES",
			Passed:     false,
			Difference: fmt.Sprintf("Failed to create table: %v", err),
		})
		return results
	}
	defer func() {
		_, _ = pgDB.ExecContext(context.Background(), "DROP TABLE BENCH_CONF_TYPES")
	}()

	// Insert test data
	blobData := bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 256) // 1024 bytes
	clobData := strings.Repeat("Pure-Go-Db2-CLOB-Test-Payload-", 40)
	blobHash := sha256.Sum256(blobData)
	blobHashHex := hex.EncodeToString(blobHash[:])

	insertSQL := `
		INSERT INTO BENCH_CONF_TYPES (
			ID, C_SMALLINT, C_INT, C_BIGINT, C_REAL, C_DOUBLE, C_DECIMAL,
			C_VARCHAR, C_DECFLOAT, C_BLOB, C_CLOB, C_VARGRAPHIC
		) VALUES (
			1, 32767, 2147483647, 9223372036854775807, 3.1415, 2.718281828459, 1234567.89,
			'Universal Db2 Conformance Suite', DECFLOAT('12345678901234567890.12345678901234', 34),
			?, ?, 'Unicode / 日本語 / Português'
		)
	`
	if _, err := pgDB.ExecContext(ctx, insertSQL, blobData, clobData); err != nil {
		results = append(results, ConformanceResult{
			Name:       "Insert Test Data",
			Passed:     false,
			Difference: fmt.Sprintf("Failed to insert: %v", err),
		})
		return results
	}

	// 4. Numeric and Integer Precision Check
	results = append(results, compareQueryRow(ctx, "Numeric Precision (Smallint, Int, Bigint)",
		pgDB, cgoDB,
		"SELECT C_SMALLINT, C_INT, C_BIGINT FROM BENCH_CONF_TYPES WHERE ID = 1",
		func(row *sql.Row) (string, error) {
			var s int16
			var i int32
			var b int64
			err := row.Scan(&s, &i, &b)
			return fmt.Sprintf("s=%d, i=%d, b=%d", s, i, b), err
		},
	))

	// 5. Decimals & DECFLOAT
	results = append(results, compareQueryRow(ctx, "High Precision Decimal & DECFLOAT",
		pgDB, cgoDB,
		"SELECT VARCHAR(C_DECIMAL), VARCHAR(C_DECFLOAT) FROM BENCH_CONF_TYPES WHERE ID = 1",
		func(row *sql.Row) (string, error) {
			var dec, decfloat string
			err := row.Scan(&dec, &decfloat)
			return fmt.Sprintf("dec=%s, decfloat=%s", strings.TrimSpace(dec), strings.TrimSpace(decfloat)), err
		},
	))

	// 6. BLOB Hash Verification
	results = append(results, compareQueryRow(ctx, "BLOB Binary Integrity (SHA-256)",
		pgDB, cgoDB,
		"SELECT C_BLOB FROM BENCH_CONF_TYPES WHERE ID = 1",
		func(row *sql.Row) (string, error) {
			var b []byte
			err := row.Scan(&b)
			h := sha256.Sum256(b)
			return hex.EncodeToString(h[:]), err
		},
	))
	// Verify blob hash matches source
	if len(results) > 0 && results[len(results)-1].PureGoVal != blobHashHex {
		results[len(results)-1].Passed = false
		results[len(results)-1].Difference = fmt.Sprintf("Expected hash %s, got %s", blobHashHex, results[len(results)-1].PureGoVal)
	}

	// 7. CLOB String Length and Content
	results = append(results, compareQueryRow(ctx, "CLOB Text Streaming",
		pgDB, cgoDB,
		"SELECT LENGTH(C_CLOB), SUBSTR(C_CLOB, 1, 30) FROM BENCH_CONF_TYPES WHERE ID = 1",
		func(row *sql.Row) (string, error) {
			var length int
			var prefix string
			err := row.Scan(&length, &prefix)
			return fmt.Sprintf("len=%d, prefix=%s", length, prefix), err
		},
	))

	// 8. Graphic / Unicode Strings
	results = append(results, compareQueryRow(ctx, "VARGRAPHIC International Strings",
		pgDB, cgoDB,
		"SELECT C_VARGRAPHIC FROM BENCH_CONF_TYPES WHERE ID = 1",
		func(row *sql.Row) (string, error) {
			var s string
			err := row.Scan(&s)
			return strings.TrimSpace(s), err
		},
	))

	return results
}

func compareQueryRow(ctx context.Context, name string, pgDB, cgoDB *sql.DB, query string, scanFn func(*sql.Row) (string, error)) ConformanceResult {
	pgRow := pgDB.QueryRowContext(ctx, query)
	pgVal, pgErr := scanFn(pgRow)
	if pgErr != nil {
		return ConformanceResult{
			Name:       name,
			PureGoVal:  fmt.Sprintf("ERR: %v", pgErr),
			Passed:     false,
			Difference: fmt.Sprintf("go-db2 query error: %v", pgErr),
		}
	}

	cgoRow := cgoDB.QueryRowContext(ctx, query)
	cgoVal, cgoErr := scanFn(cgoRow)
	if cgoErr != nil {
		return ConformanceResult{
			Name:       name,
			PureGoVal:  pgVal,
			CgoVal:     fmt.Sprintf("ERR: %v", cgoErr),
			Passed:     false,
			Difference: fmt.Sprintf("go_ibm_db query error: %v", cgoErr),
		}
	}

	passed := strings.TrimSpace(pgVal) == strings.TrimSpace(cgoVal)
	diff := ""
	if !passed {
		diff = fmt.Sprintf("go-db2 (%q) != go_ibm_db (%q)", pgVal, cgoVal)
	}

	return ConformanceResult{
		Name:       name,
		PureGoVal:  pgVal,
		CgoVal:     cgoVal,
		Passed:     passed,
		Difference: diff,
	}
}
