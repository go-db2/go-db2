package suite

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"time"
)

// RunPerformanceSuite runs head-to-head performance benchmarks.
func RunPerformanceSuite(ctx context.Context, pgDB, cgoDB *sql.DB) []BenchmarkResult {
	var results []BenchmarkResult

	// Setup benchmark tables
	_, _ = pgDB.ExecContext(ctx, "DROP TABLE BENCH_PERF_ROWS")
	_, _ = pgDB.ExecContext(ctx, "DROP TABLE BENCH_PERF_INSERT")
	_, _ = pgDB.ExecContext(ctx, "DROP TABLE BENCH_PERF_LOB")

	_, _ = pgDB.ExecContext(ctx, "CREATE TABLE BENCH_PERF_ROWS (ID INT NOT NULL PRIMARY KEY, NAME VARCHAR(50), VAL DOUBLE)")
	_, _ = pgDB.ExecContext(ctx, "CREATE TABLE BENCH_PERF_INSERT (ID INT NOT NULL PRIMARY KEY, NAME VARCHAR(50), VAL DOUBLE)")
	_, _ = pgDB.ExecContext(ctx, "CREATE TABLE BENCH_PERF_LOB (ID INT NOT NULL PRIMARY KEY, DATA BLOB(1M))")

	defer func() {
		_, _ = pgDB.ExecContext(context.Background(), "DROP TABLE BENCH_PERF_ROWS")
		_, _ = pgDB.ExecContext(context.Background(), "DROP TABLE BENCH_PERF_INSERT")
		_, _ = pgDB.ExecContext(context.Background(), "DROP TABLE BENCH_PERF_LOB")
	}()

	// Populate 1,000 rows in BENCH_PERF_ROWS
	stmt, err := pgDB.PrepareContext(ctx, "INSERT INTO BENCH_PERF_ROWS (ID, NAME, VAL) VALUES (?, ?, ?)")
	if err == nil {
		for i := 1; i <= 1000; i++ {
			_, _ = stmt.ExecContext(ctx, i, fmt.Sprintf("Customer-Record-%05d", i), float64(i)*1.5)
		}
		stmt.Close()
	}

	// Populate 500KB BLOB in BENCH_PERF_LOB
	blobPayload := bytes.Repeat([]byte("PureGoDb2DriverBenchmarkPayload1234567890!"), 12000) // ~500 KB
	_, _ = pgDB.ExecContext(ctx, "INSERT INTO BENCH_PERF_LOB (ID, DATA) VALUES (1, ?)", blobPayload)

	// 1. Benchmark: Connection Ping Latency
	results = append(results, runComparison("Ping (Round-trip Latency)", 500, func(db *sql.DB) error {
		return db.PingContext(ctx)
	}, pgDB, cgoDB))

	// 2. Benchmark: Simple Single-Row Select
	results = append(results, runComparison("Simple Single-Row SELECT", 1000, func(db *sql.DB) error {
		var val int
		return db.QueryRowContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1").Scan(&val)
	}, pgDB, cgoDB))

	// 3. Benchmark: Multi-Row Fetch (1,000 rows iteration)
	results = append(results, runComparison("Fetch 1,000 Rows Scan", 50, func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, "SELECT ID, NAME, VAL FROM BENCH_PERF_ROWS")
		if err != nil {
			return err
		}
		defer rows.Close()

		var id int
		var name string
		var val float64
		count := 0
		for rows.Next() {
			if err := rows.Scan(&id, &name, &val); err != nil {
				return err
			}
			count++
		}
		return rows.Err()
	}, pgDB, cgoDB))

	// 4. Benchmark: Prepared Statement DML Exec
	results = append(results, runComparison("Prepared Statement INSERT", 300, func(db *sql.DB) error {
		// Clean table before test
		_, _ = db.ExecContext(ctx, "DELETE FROM BENCH_PERF_INSERT")
		stmt, err := db.PrepareContext(ctx, "INSERT INTO BENCH_PERF_INSERT (ID, NAME, VAL) VALUES (?, ?, ?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for i := 1; i <= 10; i++ {
			if _, err := stmt.ExecContext(ctx, i, "test_name", float64(i)*2.5); err != nil {
				return err
			}
		}
		return nil
	}, pgDB, cgoDB))

	// 5. Benchmark: 500KB LOB Streaming Read
	results = append(results, runComparison("Read 500KB BLOB Payload", 50, func(db *sql.DB) error {
		var data []byte
		return db.QueryRowContext(ctx, "SELECT DATA FROM BENCH_PERF_LOB WHERE ID = 1").Scan(&data)
	}, pgDB, cgoDB))

	return results
}

func runComparison(name string, iterations int, fn func(*sql.DB) error, pgDB, cgoDB *sql.DB) BenchmarkResult {
	pgMetric := measure(iterations, func() error { return fn(pgDB) })
	cgoMetric := measure(iterations, func() error { return fn(cgoDB) })

	return BenchmarkResult{
		Name:   name,
		PureGo: pgMetric,
		CgoIBM: cgoMetric,
	}
}

func measure(iterations int, fn func() error) PerfMetric {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = fn()
	}
	duration := time.Since(start)

	runtime.ReadMemStats(&m2)

	bytesAlloc := uint64(0)
	if m2.TotalAlloc > m1.TotalAlloc {
		bytesAlloc = m2.TotalAlloc - m1.TotalAlloc
	}

	allocs := uint64(0)
	if m2.Mallocs > m1.Mallocs {
		allocs = m2.Mallocs - m1.Mallocs
	}

	return PerfMetric{
		Duration:   duration,
		Ops:        iterations,
		BytesAlloc: bytesAlloc,
		Allocs:     allocs,
	}
}
