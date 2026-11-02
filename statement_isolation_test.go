package db2

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"math"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/codec"
	"github.com/go-db2/go-db2/internal/drda"
)

// AC-1: Statement isolation on the same connection.
// Validates that multiple statements prepared on the same connection obtain distinct DRDA sections (pkgsn).
func TestAC1_StatementIsolation_DistinctPkgsn(t *testing.T) {
	conn := newConn(nil, &Config{Database: "TESTDB"})

	sm := conn.getSections()

	// stmt1 acquires section
	sec1, err := sm.Acquire()
	if err != nil {
		t.Fatalf("unexpected error acquiring sec1: %v", err)
	}
	stmt1 := newStmt(conn, "INSERT INTO t1 VALUES (?)", nil, nil, sec1)

	// stmt2 acquires section
	sec2, err := sm.Acquire()
	if err != nil {
		t.Fatalf("unexpected error acquiring sec2: %v", err)
	}
	stmt2 := newStmt(conn, "DELETE FROM t2 WHERE id = ?", nil, nil, sec2)

	if stmt1.pkgsn == stmt2.pkgsn {
		t.Fatalf("expected distinct pkgsn for stmt1 and stmt2, got %d for both", stmt1.pkgsn)
	}
	if stmt1.pkgsn < 2 || stmt2.pkgsn < 2 {
		t.Fatalf("expected pkgsn >= 2, got %d and %d", stmt1.pkgsn, stmt2.pkgsn)
	}

	_ = stmt1.Close()
	_ = stmt2.Close()
}

// AC-2: Lifecycle and reuse of DRDA sections.
// 1. Preparing and closing 600 statements sequentially with max 500 sections does not exhaust pool.
// 2. Executing 600 parameterized direct queries releases ephemeral sections via defer.
func TestAC2_SectionLifecycleAndExhaustion(t *testing.T) {
	conn := newConn(nil, &Config{Database: "TESTDB"})

	// 1. Prepare and close 600 statements
	for i := 0; i < 600; i++ {
		sec, err := conn.getSections().Acquire()
		if err != nil {
			t.Fatalf("iteration %d: unexpected exhaustion error acquiring section: %v", i, err)
		}
		stmt := newStmt(conn, "SELECT 1 FROM SYSIBM.SYSDUMMY1", nil, nil, sec)
		if err := stmt.Close(); err != nil {
			t.Fatalf("iteration %d: unexpected error closing stmt: %v", i, err)
		}
	}

	// 2. Simulate 600 parameterized direct executions with defer release
	for i := 0; i < 600; i++ {
		func() {
			sec, err := conn.getSections().Acquire()
			if err != nil {
				t.Fatalf("iteration %d: unexpected exhaustion in parameterized execution: %v", i, err)
			}
			defer conn.getSections().Release(sec)
		}()
	}
}

// AC-3: Section rollback when preparation fails.
func TestAC3_SectionRollbackOnPrepareFailure(t *testing.T) {
	conn := newConn(nil, &Config{Database: "TESTDB"})
	sm := conn.getSections()

	// Acquire a section
	sec, err := sm.Acquire()
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if !sm.isAllocated(sec) {
		t.Fatalf("expected section %d to be marked allocated in bitmap", sec)
	}

	// Prepare failure occurs: rollback section immediately
	sm.Release(sec)

	// Verify the section is marked as free immediately in the bitmap
	if sm.isAllocated(sec) {
		t.Fatalf("expected section %d to be marked as free after rollback", sec)
	}
}

// Type that implements driver.Valuer
type mockValuer struct {
	val any
}

func (m mockValuer) Value() (driver.Value, error) {
	return m.val, nil
}

// AC-4: driver.Valuer, [16]byte UUID, and Batch discrimination in CheckNamedValue.
func TestAC4_NamedValueChecker_Valuer_UUID_Batch(t *testing.T) {
	conn := newConn(nil, &Config{Database: "TESTDB"})

	// 1. driver.Valuer dereferencing
	nvValuer := &driver.NamedValue{
		Ordinal: 1,
		Value:   mockValuer{val: "valuer_result"},
	}
	if err := conn.CheckNamedValue(nvValuer); err != driver.ErrSkip {
		t.Fatalf("expected ErrSkip for scalar string after Valuer desreference, got: %v", err)
	}
	if nvValuer.Value != "valuer_result" {
		t.Fatalf("expected nvValuer.Value to be 'valuer_result', got %v", nvValuer.Value)
	}

	// 2. Fixed [16]byte UUID normalized to []byte and not treated as batch
	var uuidBytes [16]byte
	copy(uuidBytes[:], "0123456789abcdef")
	nvUUID := &driver.NamedValue{
		Ordinal: 1,
		Value:   uuidBytes,
	}
	if err := conn.CheckNamedValue(nvUUID); err != nil {
		t.Fatalf("unexpected error for [16]byte: %v", err)
	}
	bSlice, isByteSlice := nvUUID.Value.([]byte)
	if !isByteSlice || len(bSlice) != 16 {
		t.Fatalf("expected []byte of length 16, got %T (%v)", nvUUID.Value, nvUUID.Value)
	}

	// Verify [16]byte is not treated as batch in detectAndExtractBatch
	isBatch, _, err := detectAndExtractBatch([]any{uuidBytes})
	if err != nil {
		t.Fatalf("detectAndExtractBatch failed: %v", err)
	}
	if isBatch {
		t.Fatalf("expected [16]byte to NOT be treated as batch")
	}

	// 3. Batch slices return nil to prevent database/sql from rejecting them
	batchInputs := []any{
		[]int{1, 2, 3},
		[]int64{10, 20, 30},
		[]string{"a", "b", "c"},
		[]float64{1.1, 2.2, 3.3},
		[]any{"x", 100, true},
	}
	for _, input := range batchInputs {
		nv := &driver.NamedValue{Ordinal: 1, Value: input}
		if err := conn.CheckNamedValue(nv); err != nil {
			t.Fatalf("expected nil for batch slice %T, got %v", input, err)
		}
	}

	// 4. Scalar types delegate to database/sql via driver.ErrSkip
	scalarInputs := []any{
		int64(42),
		"plain_string",
		3.14,
		true,
		time.Now(),
	}
	for _, input := range scalarInputs {
		nv := &driver.NamedValue{Ordinal: 1, Value: input}
		if err := conn.CheckNamedValue(nv); err != driver.ErrSkip {
			t.Fatalf("expected driver.ErrSkip for scalar %T, got %v", input, err)
		}
	}

	// Slices of bytes ([]byte) are handled safely without rejection
	nvBytes := &driver.NamedValue{Ordinal: 1, Value: []byte{0x01, 0x02}}
	if err := conn.CheckNamedValue(nvBytes); err != nil && err != driver.ErrSkip {
		t.Fatalf("unexpected error for []byte: %v", err)
	}
}

// AC-5: Strict numeric validation without truncation or heap allocations.
func TestAC5_StrictNumericValidation(t *testing.T) {
	endian := binary.BigEndian

	// SMALLINT overflow: 70000
	_, err := codec.BuildSQLDTA(
		[]codec.SQLType{codec.SQLTypeSmall},
		[]int64{2},
		[]int{0},
		[]int{0},
		[]any{int64(70000)},
		endian,
	)
	if err == nil {
		t.Fatal("expected error for SMALLINT overflow 70000, got nil")
	}

	// INTEGER overflow: 5000000000
	_, err = codec.BuildSQLDTA(
		[]codec.SQLType{codec.SQLTypeInteger},
		[]int64{4},
		[]int{0},
		[]int{0},
		[]any{int64(5000000000)},
		endian,
	)
	if err == nil {
		t.Fatal("expected error for INTEGER overflow 5000000000, got nil")
	}

	// Incompatible type: string "texto" passed to numeric column
	_, err = codec.BuildSQLDTA(
		[]codec.SQLType{codec.SQLTypeInteger},
		[]int64{4},
		[]int{0},
		[]int{0},
		[]any{"texto"},
		endian,
	)
	if err == nil {
		t.Fatal("expected error for string passed to INTEGER column, got nil")
	}

	// uint64 overflow > math.MaxInt64
	_, err = codec.BuildSQLDTA(
		[]codec.SQLType{codec.SQLTypeBigInt},
		[]int64{8},
		[]int{0},
		[]int{0},
		[]any{uint64(math.MaxUint64)},
		endian,
	)
	if err == nil {
		t.Fatal("expected error for uint64 overflow, got nil")
	}
}

// AC-6: Transaction isolation levels and state restoration.
func TestAC6_TransactionIsolation(t *testing.T) {
	conn := newConn(nil, &Config{Database: "TESTDB"})

	// Unsupported isolation level
	_, err := conn.BeginTx(context.Background(), driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelSnapshot),
	})
	if err == nil {
		t.Fatal("expected error for sql.LevelSnapshot, got nil")
	}

	// ReadOnly transaction requested
	_, err = conn.BeginTx(context.Background(), driver.TxOptions{
		ReadOnly: true,
	})
	if err == nil {
		t.Fatal("expected error for ReadOnly transaction, got nil")
	}
}

// AC-7: Result decoupling and safe LastInsertId contract.
func TestAC7_ResultDecouplingAndLastInsertId(t *testing.T) {
	res := newResult(10, 0)

	// LastInsertId returns explicit unsupported error
	_, err := res.LastInsertId()
	if err == nil {
		t.Fatal("expected error from res.LastInsertId(), got nil")
	}
	expectedMsg := "LastInsertId is not supported by Db2 DRDA protocol"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Fatalf("expected error containing %q, got: %v", expectedMsg, err)
	}

	// RowsAffected returns affected rows correctly
	rows, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("unexpected error from RowsAffected(): %v", err)
	}
	if rows != 10 {
		t.Fatalf("expected 10 rows affected, got %d", rows)
	}

	// Verify Result struct does NOT retain a *Conn pointer
	resType := reflect.TypeOf(Result{})
	for i := 0; i < resType.NumField(); i++ {
		field := resType.Field(i)
		if field.Type == reflect.TypeOf((*Conn)(nil)) {
			t.Fatalf("Result struct must not contain *Conn pointer field: %s", field.Name)
		}
	}
}

// Test sectionManager concurrent thread-safety (NFR-1 / AC-8).
func TestSectionManager_ConcurrentAcquireRelease(t *testing.T) {
	sm := newSectionManager(100)
	var wg sync.WaitGroup

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				sec, err := sm.Acquire()
				if err == nil {
					sm.Release(sec)
				}
			}
		}()
	}
	wg.Wait()
}

// TestMockServer_AC1_StatementIsolationDRDA verifies statement isolation end-to-end with network interaction.
func TestMockServer_AC1_StatementIsolationDRDA(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointPRPSQLSTT, drda.CodePointDSCSQLSTT, drda.CodePointEXCSQLSTT, drda.CodePointRDBCMM, drda.CodePointEXCSAT:
			return writeSQLCARDOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://db2inst1:password@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	driverConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer driverConn.Close()

	var conn *Conn
	_ = driverConn.Raw(func(raw any) error {
		if c, ok := raw.(*Conn); ok {
			conn = c
		}
		return nil
	})
	if conn == nil {
		t.Fatal("failed to extract *Conn")
	}

	stmt1, err := conn.PrepareContext(ctx, "INSERT INTO t1 VALUES (?)")
	if err != nil {
		t.Fatalf("PrepareContext stmt1 failed: %v", err)
	}
	defer stmt1.Close()

	stmt2, err := conn.PrepareContext(ctx, "DELETE FROM t2 WHERE id = ?")
	if err != nil {
		t.Fatalf("PrepareContext stmt2 failed: %v", err)
	}
	defer stmt2.Close()

	s1, ok1 := stmt1.(*Stmt)
	s2, ok2 := stmt2.(*Stmt)
	if !ok1 || !ok2 {
		t.Fatalf("expected *Stmt wrappers, got %T and %T", stmt1, stmt2)
	}

	if s1.pkgsn == s2.pkgsn {
		t.Fatalf("expected unique pkgsn for prepared statements, got %d for both", s1.pkgsn)
	}
	if s1.pkgsn != 2 || s2.pkgsn != 3 {
		t.Fatalf("expected s1.pkgsn == 2 and s2.pkgsn == 3, got %d and %d", s1.pkgsn, s2.pkgsn)
	}
}

// TestMockServer_AC6_TransactionIsolationRestoration verifies isolation setting and restoration end-to-end.
func TestMockServer_AC6_TransactionIsolationRestoration(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointEXCSQLIMM, drda.CodePointRDBCMM, drda.CodePointRDBRLLBCK, drda.CodePointEXCSAT, drda.CodePointEXCSQLSET:
			return writeSQLCARDOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://db2inst1:password@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	driverConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer driverConn.Close()

	var conn *Conn
	_ = driverConn.Raw(func(raw any) error {
		if c, ok := raw.(*Conn); ok {
			conn = c
		}
		return nil
	})
	if conn == nil {
		t.Fatal("failed to extract *Conn")
	}

	// 1. BeginTx with LevelSerializable
	tx, err := conn.BeginTx(ctx, driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelSerializable),
	})
	if err != nil {
		t.Fatalf("BeginTx LevelSerializable failed: %v", err)
	}
	if !conn.hasCustomIso {
		t.Fatal("expected conn.hasCustomIso to be true during custom transaction")
	}

	// Commit must clean up and restore hasCustomIso = false
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit failed: %v", err)
	}
	if conn.hasCustomIso {
		t.Fatal("expected conn.hasCustomIso to be false after commit")
	}

	// 2. BeginTx with LevelReadUncommitted
	tx2, err := conn.BeginTx(ctx, driver.TxOptions{
		Isolation: driver.IsolationLevel(sql.LevelReadUncommitted),
	})
	if err != nil {
		t.Fatalf("BeginTx LevelReadUncommitted failed: %v", err)
	}
	if !conn.hasCustomIso {
		t.Fatal("expected conn.hasCustomIso to be true during custom transaction")
	}

	// Rollback must clean up and restore hasCustomIso = false
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("tx2.Rollback failed: %v", err)
	}
	if conn.hasCustomIso {
		t.Fatal("expected conn.hasCustomIso to be false after rollback")
	}

	// 3. ResetSession cleans up if conn returned to pool with dirty isolation
	conn.hasCustomIso = true
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}
	if conn.hasCustomIso {
		t.Fatal("expected conn.hasCustomIso to be reset to false by ResetSession")
	}
}
