package db2

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/auth"
	"github.com/go-db2/go-db2/internal/codec"
	"github.com/go-db2/go-db2/internal/drda"
)

// 1. SEC-01: SQL Injection in Administrative Functions (CreateDb, DropDb)
func TestSecurity_SQLInjection_AdminFunctions(t *testing.T) {
	maliciousPayloads := []string{
		"TESTDB; DROP TABLE USERS; --",
		"DB' OR '1'='1",
		"SAMPLE; SHUTDOWN;",
		"DB\x00INJECT",
		"DB NAME WITH SPACES",
		"DB/SLASH",
		"DB#INJECTION'--",
		"",
	}

	dummyConnStr := "db2://dummy:dummy@localhost:50000/DUMMY"

	for _, payload := range maliciousPayloads {
		t.Run("CreateDb_"+payload, func(t *testing.T) {
			_, err := CreateDb(payload, dummyConnStr)
			if err == nil {
				t.Fatalf("expected error for malicious dbname %q in CreateDb, got nil", payload)
			}
		})

		t.Run("DropDb_"+payload, func(t *testing.T) {
			_, err := DropDb(payload, dummyConnStr)
			if err == nil {
				t.Fatalf("expected error for malicious dbname %q in DropDb, got nil", payload)
			}
		})
	}
}

// 2. SEC-02: SQL Injection in SwitchUser
func TestSecurity_SQLInjection_SwitchUser(t *testing.T) {
	maliciousUsers := []string{
		"ADMIN; DROP TABLE USERS; --",
		"USER' OR '1'='1",
		"ROOT\x00ADMIN",
		"USER WITH SPACES",
		"USER-MINUS",
		"",
	}

	session := &drda.Session{}

	for _, user := range maliciousUsers {
		t.Run("SwitchUser_"+user, func(t *testing.T) {
			err := session.SwitchUser(context.Background(), user)
			if err == nil {
				t.Fatalf("expected error for malicious user %q in SwitchUser, got nil", user)
			}
		})
	}
}

// 3. SEC-13: Password Redaction in Config & SessionConfig
func TestSecurity_PasswordRedaction(t *testing.T) {
	secretPwd := "SuperSecretP@ssw0rd!123"

	cfg := &Config{
		Host:     "db2host.corp",
		Port:     50000,
		Database: "PRODDB",
		User:     "admin",
		Password: secretPwd,
		UseSSL:   true,
		Timeout:  30 * time.Second,
	}

	strOutput := fmt.Sprintf("%v", cfg)
	strDetailed := fmt.Sprintf("%+v", cfg)
	strGo := fmt.Sprintf("%#v", cfg)

	for _, out := range []string{strOutput, strDetailed, strGo} {
		if strings.Contains(out, secretPwd) {
			t.Fatalf("Config leaked plaintext password in log output: %s", out)
		}
		if !strings.Contains(out, "******") {
			t.Fatalf("Config did not redact password with '******' in log output: %s", out)
		}
	}

	sessCfg := cfg.toSessionConfig()
	sStrOutput := fmt.Sprintf("%v", sessCfg)
	sStrDetailed := fmt.Sprintf("%+v", sessCfg)

	for _, out := range []string{sStrOutput, sStrDetailed} {
		if strings.Contains(out, secretPwd) {
			t.Fatalf("SessionConfig leaked plaintext password in log output: %s", out)
		}
		if !strings.Contains(out, "******") {
			t.Fatalf("SessionConfig did not redact password with '******' in log output: %s", out)
		}
	}
}

// 4. SEC-04: Support for >84 Parameters without Integer Overflow
func TestSecurity_BuildSQLDTA_Over84Parameters(t *testing.T) {
	numParams := 100 // Exceeds the 84 limit that would overflow a uint8

	colTypes := make([]codec.SQLType, numParams)
	colLens := make([]int64, numParams)
	precs := make([]int, numParams)
	scales := make([]int, numParams)
	args := make([]any, numParams)

	for i := 0; i < numParams; i++ {
		colTypes[i] = codec.SQLTypeInteger
		colLens[i] = 4
		precs[i] = 0
		scales[i] = 0
		args[i] = int32(i + 1)
	}

	sqldta, err := codec.BuildSQLDTA(colTypes, colLens, precs, scales, args, binary.BigEndian)
	if err != nil {
		t.Fatalf("BuildSQLDTA failed for %d parameters: %v", numParams, err)
	}

	if len(sqldta) < 8 {
		t.Fatalf("BuildSQLDTA produced too short buffer: %d bytes", len(sqldta))
	}

	totalLen := binary.BigEndian.Uint16(sqldta[0:2])
	if int(totalLen) != len(sqldta) {
		t.Fatalf("BuildSQLDTA object length mismatch: header=%d, actual=%d", totalLen, len(sqldta))
	}
}

// 5. SEC-03: SQL Parser Rewriting with Comments and String Literals
func TestSecurity_RewriteBinaryParams_WithCommentsAndStrings(t *testing.T) {
	query := "SELECT 'Is this a param ? or not?' AS txt, col1 FROM tbl WHERE id = ? -- is this id ?\n AND name = /* block ? */ ?"
	paramCols := []drda.ColumnDescription{
		{SQLType: uint16(codec.SQLTypeInteger)},
		{SQLType: uint16(codec.SQLTypeVarChar)},
	}
	args := []any{100, "Alice"}

	rewrittenQuery, newCols, newArgs := rewriteBinaryParams(query, paramCols, args)

	if len(newCols) != 2 || len(newArgs) != 2 {
		t.Fatalf("expected exactly 2 parameters extracted, got cols=%d args=%d", len(newCols), len(newArgs))
	}

	if !strings.Contains(rewrittenQuery, "'Is this a param ? or not?'") {
		t.Errorf("string literal was corrupted: %s", rewrittenQuery)
	}
}

// 6. SEC-08: Diffie-Hellman Small-Subgroup Validation in SECMEC 9
func TestSecurity_SECMEC9_InvalidDHPublicKeys(t *testing.T) {
	clientPriv, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	clientPriv.Add(clientPriv, big.NewInt(2))

	// Weak/Invalid Public Keys: 0, 1, and p-1
	zeroKey := make([]byte, 32) // 0
	oneKey := make([]byte, 32)
	oneKey[31] = 0x01 // 1

	pMinusOne := new(big.Int).Sub(auth.DHSecmec9Prime, big.NewInt(1))
	pMinusOneKey := pMinusOne.Bytes()

	testCases := []struct {
		name string
		key  []byte
	}{
		{"ZeroKey", zeroKey},
		{"OneKey", oneKey},
		{"PMinusOneKey", pMinusOneKey},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.CalculateSessionKey(tc.key, clientPriv)
			if err == nil {
				t.Fatalf("expected error for weak DH public key %s, got nil", tc.name)
			}
		})
	}
}

// 7. SEC-14: Explicit Error on Invalid Temporal Format in toTime
func TestSecurity_FDODTA_InvalidTemporalFormat(t *testing.T) {
	invalidDates := []string{
		"invalid-date-string",
		"99/99/9999",
		"not a date",
	}

	for _, invalid := range invalidDates {
		_, err := codec.FDODTA(codec.SQLTypeDate, 10, 0, 0, invalid, binary.BigEndian)
		if err == nil {
			t.Fatalf("expected error for invalid date %q, got nil", invalid)
		}
	}
}

// 8. SEC-10 & SEC-11: Mutex Concurrency on Stmt and Result
func TestSecurity_Stmt_And_Result_Concurrency(t *testing.T) {
	conn := &Conn{}
	stmt := newStmt(conn, "SELECT 1 FROM SYSIBM.SYSDUMMY1 WHERE ID = ?", nil, []drda.ColumnDescription{
		{SQLType: uint16(codec.SQLTypeInteger)},
	})

	var wg sync.WaitGroup
	numWorkers := 30

	// Test Close and state checking concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = stmt.NumInput()
			res := newResult(int64(id), int64(id*10))
			_, _ = res.RowsAffected()
		}(i)
	}

	wg.Wait()
	_ = stmt.Close()
}

// 8b. SEC-06: Concurrent Stmt.NumInput(), ExecContext (BLOB rewrite + scalar restore), QueryContext, and Close under -race
func TestSecurity_Stmt_ConcurrentNumInputAndInlineLOB(t *testing.T) {
	// Build a valid LittleEndian input-parameter SQLDARD payload describing 2 params: [BLOB(1024), INTEGER(4)]
	var sqldard []byte
	sqldard = append(sqldard, 0xFF, 0xFF) // input paramCols indicator + unnamed header marker
	sqldard = binary.LittleEndian.AppendUint16(sqldard, 2)
	for _, col := range []struct {
		sqlType uint16
		length  uint64
	}{
		{uint16(codec.SQLTypeBlob), 1024},
		{uint16(codec.SQLTypeInteger), 4},
	} {
		sqldard = binary.LittleEndian.AppendUint16(sqldard, 0) // precision
		sqldard = binary.LittleEndian.AppendUint16(sqldard, 0) // scale
		sqldard = binary.LittleEndian.AppendUint64(sqldard, col.length)
		sqldard = binary.LittleEndian.AppendUint16(sqldard, col.sqlType)
		sqldard = append(sqldard, 0x00, 0x00)
		sqldard = append(sqldard, make([]byte, 29)...)
	}

	sqlcardOK := make([]byte, 20)
	binary.LittleEndian.PutUint32(sqlcardOK[1:5], 0)
	copy(sqlcardOK[5:10], "00000")

	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointPRPSQLSTT:
			cardObj := drda.PackDDMObject(drda.CodePointSQLCARD, sqlcardOK)
			dardObj := drda.PackDDMObject(drda.CodePointSQLDARD, sqldard)
			var resp []byte
			resp = append(resp, drda.BuildDSSHeader(len(cardObj), drda.DSSTypeReply, true, false, false, 1)...)
			resp = append(resp, cardObj...)
			resp = append(resp, drda.BuildDSSHeader(len(dardObj), drda.DSSTypeReply, false, false, false, 1)...)
			resp = append(resp, dardObj...)
			_, err := c.Write(resp)
			return err == nil
		case drda.CodePointOPNQRY:
			_, err := c.Write(buildReplyChain(
				mockOpnqryrm(),
				mockQrydscInteger(),
				mockQrydta(mockIntegerRows(42)),
				mockSqlcard(0, "00000"),
			))
			return err == nil
		case drda.CodePointEXCSQLIMM, drda.CodePointEXCSQLSTT, drda.CodePointRDBCMM, drda.CodePointRDBRLLBCK, drda.CodePointCLSQRY:
			return writeSQLCARDOK(c)
		}
		return true
	})

	db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sqlConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer sqlConn.Close()

	var conn *Conn
	if err := sqlConn.Raw(func(raw any) error {
		conn = raw.(*Conn)
		return nil
	}); err != nil || conn == nil {
		t.Fatalf("failed to extract *Conn: %v", err)
	}

	stmt := newStmt(conn, "UPDATE T SET B = ? WHERE ID = ?", nil, []drda.ColumnDescription{
		{SQLType: uint16(codec.SQLTypeBlob), Length: 1024},
		{SQLType: uint16(codec.SQLTypeInteger), Length: 4},
	}, 1)

	sec2, err := conn.sections.Acquire()
	if err != nil {
		t.Fatalf("failed to acquire section for qStmt: %v", err)
	}
	qStmt := newStmt(conn, "SELECT ID FROM T WHERE ID = ?", []drda.ColumnDescription{
		{Name: "ID", SQLType: uint16(codec.SQLTypeInteger), Length: 4},
	}, []drda.ColumnDescription{
		{SQLType: uint16(codec.SQLTypeInteger), Length: 4},
	}, sec2)

	var wg sync.WaitGroup
	triggerClose := make(chan struct{})
	var closeOnce sync.Once

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-triggerClose
		_ = stmt.Close()
		_ = qStmt.Close()
	}()

	// Concurrent NumInput readers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = stmt.NumInput()
				_ = qStmt.NumInput()
			}
		}()
	}

	// Concurrent ExecContext (alternating BLOB inline rewrite and batch/scalar restore) and QueryContext (both exec-path and streaming outputCols path)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			if worker == 0 {
				defer closeOnce.Do(func() { close(triggerClose) })
			}
			for j := 0; j < 20; j++ {
				if worker == 0 && j == 12 {
					closeOnce.Do(func() { close(triggerClose) })
				}
				if j%2 == 0 {
					_, _ = stmt.ExecContext(ctx, []driver.NamedValue{
						{Ordinal: 1, Value: []byte{0xde, 0xad, 0xbe, 0xef}},
						{Ordinal: 2, Value: int64(worker*100 + j)},
					})
				} else {
					rows, qErr := stmt.QueryContext(ctx, []driver.NamedValue{
						{Ordinal: 1, Value: []byte{}},
						{Ordinal: 2, Value: int64(j)},
					})
					if qErr == nil && rows != nil {
						_ = rows.Close()
					}
					qRows, qsErr := qStmt.QueryContext(ctx, []driver.NamedValue{
						{Ordinal: 1, Value: int64(worker*100 + j)},
					})
					if qsErr == nil && qRows != nil {
						_ = qRows.Close()
					}
				}
			}
		}(i)
	}

	wg.Wait()
	if err := stmt.Close(); err != nil {
		t.Fatalf("stmt.Close failed: %v", err)
	}
	if err := qStmt.Close(); err != nil {
		t.Fatalf("qStmt.Close failed: %v", err)
	}
}

// 9. SEC-02: ReadDSS bounds checking on malformed frame length
func TestSecurity_ReadDSS_MalformedFrameLength(t *testing.T) {
	// Construct a 20-byte DSS frame claiming to have a 50-byte DDM object
	var buf []byte
	// DSS Header (6 bytes): totalLen=20, magic=0xD0, flags=0x01, corrID=1
	buf = append(buf, 0x00, 20, 0xD0, 0x01, 0x00, 0x01)
	// DDM Object Header (4 bytes): objLen=50 (exceeds frame!), cp=0x2001
	buf = append(buf, 0x00, 50, 0x20, 0x01)
	// Remaining 10 bytes to satisfy the 20-byte frame
	buf = append(buf, make([]byte, 10)...)

	reader := strings.NewReader(string(buf))
	_, _, _, _, err := drda.ReadDSS(reader)
	if err == nil {
		t.Fatal("expected error when objLen > dssLen - 6, got nil")
	}
}

// 10. SEC-03: DecodeField panic protection for truncated metadata (len(ps) < 2)
func TestSecurity_DecodeField_TruncatedMetadataPanicProtection(t *testing.T) {
	testTypes := []uint8{
		codec.DRDATypeChar,
		codec.DRDATypeGraphic,
		codec.DRDATypeVarChar,
		codec.DRDATypeDate,
		codec.DRDATypeTime,
		codec.DRDATypeTimestamp,
		codec.DRDATypeDecimal,
	}

	invalidPSList := [][]byte{
		nil,
		{},
		{0x01},
	}

	for _, dt := range testTypes {
		for _, ps := range invalidPSList {
			t.Run(fmt.Sprintf("Type_0x%02X_PSLen_%d", dt, len(ps)), func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("DecodeField panicked on truncated ps: %v", r)
					}
				}()

				dummyData := strings.NewReader("1234567890abcdef")
				_, err := codec.DecodeField(dt, ps, dummyData, binary.LittleEndian)
				if err == nil {
					t.Fatalf("expected error for truncated ps with len %d, got nil", len(ps))
				}
			})
		}
	}
}

// 11. SEC-07: PackDDMObject protection against uint16 overflow: a body too
// long for a 15-bit length is carried whole behind a 4-byte extended length.
func TestSecurity_PackDDMObject_LargePayloadProtection(t *testing.T) {
	largeBody := make([]byte, 70000)
	obj := drda.PackDDMObject(drda.CodePointSQLSTT, largeBody)
	if got := binary.BigEndian.Uint16(obj[0:2]); got != 0x8008 {
		t.Fatalf("expected extended length field 0x8008, got 0x%04X", got)
	}
	if got := binary.BigEndian.Uint32(obj[4:8]); got != 70000 {
		t.Fatalf("expected extended length 70000, got %d", got)
	}
	if len(obj) != 8+70000 {
		t.Fatalf("expected %d bytes, got %d", 8+70000, len(obj))
	}
}

// 12. SEC-01 & SEC-05: Session AutoCommit state and Tx concurrency
func TestSecurity_Session_AutoCommitSwitching(t *testing.T) {
	sess := drda.NewSession(drda.SessionConfig{Database: "TESTDB"})
	if !sess.AutoCommit() {
		t.Fatal("expected AutoCommit=true on new session")
	}

	sess.SetAutoCommit(false)
	if sess.AutoCommit() {
		t.Fatal("expected AutoCommit=false after SetAutoCommit(false)")
	}

	sess.SetAutoCommit(true)
	if !sess.AutoCommit() {
		t.Fatal("expected AutoCommit=true after SetAutoCommit(true)")
	}

	conn := &Conn{session: sess}
	tx, err := conn.BeginTx(context.Background(), driver.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	if sess.AutoCommit() {
		t.Fatal("expected AutoCommit=false inside transaction")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tx.Rollback()
		}()
	}
	wg.Wait()

	if !sess.AutoCommit() {
		t.Fatal("expected AutoCommit=true after Rollback completion")
	}
}

// 13. SEC-01 Integration: Validate that tx.Rollback() does NOT persist data, and tx.Commit() does persist data.
func TestIntegration_Transaction_RollbackAndCommit(t *testing.T) {
	dsn := "db2://db2inst1:MinhaSenhaForte123@localhost:50000/TESTDB"
	db, err := sql.Open("db2", dsn)
	if err != nil {
		t.Skip("skipping integration test, cannot open db2:", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skip("skipping integration test, db2 not reachable:", err)
	}

	_, _ = db.ExecContext(ctx, "DROP TABLE test_sec_tx_iso")
	_, err = db.ExecContext(ctx, "CREATE TABLE test_sec_tx_iso (id INT NOT NULL PRIMARY KEY, name VARCHAR(50))")
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE test_sec_tx_iso")
	}()

	// 1. Begin transaction, insert row, and Rollback
	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	_, err = tx1.ExecContext(ctx, "INSERT INTO test_sec_tx_iso (id, name) VALUES (?, ?)", 100, "RolledBackUser")
	if err != nil {
		t.Fatalf("tx1.ExecContext failed: %v", err)
	}

	if err := tx1.Rollback(); err != nil {
		t.Fatalf("tx1.Rollback failed: %v", err)
	}

	// Verify row was NOT persisted
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_sec_tx_iso WHERE id = 100").Scan(&count)
	if err != nil {
		t.Fatalf("QueryRowContext failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("ACID FAILURE: Expected 0 rows after Rollback, got %d (auto-commit bug!)", count)
	}

	// 2. Begin transaction, insert row, and Commit
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx 2 failed: %v", err)
	}

	_, err = tx2.ExecContext(ctx, "INSERT INTO test_sec_tx_iso (id, name) VALUES (?, ?)", 200, "CommittedUser")
	if err != nil {
		t.Fatalf("tx2.ExecContext failed: %v", err)
	}

	if err := tx2.Commit(); err != nil {
		t.Fatalf("tx2.Commit failed: %v", err)
	}

	// Verify row WAS persisted
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_sec_tx_iso WHERE id = 200").Scan(&count)
	if err != nil {
		t.Fatalf("QueryRowContext 2 failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected 1 row after Commit, got %d", count)
	}
}

// 14. SEC-01 Residual: Validate that tx.ExecContext with WithClientInfo does NOT trigger auto-commit on Rollback.
func TestIntegration_Transaction_WithClientInfo_Rollback(t *testing.T) {
	dsn := "db2://db2inst1:MinhaSenhaForte123@localhost:50000/TESTDB"
	db, err := sql.Open("db2", dsn)
	if err != nil {
		t.Skip("skipping integration test, cannot open db2:", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skip("skipping integration test, db2 not reachable:", err)
	}

	_, _ = db.ExecContext(ctx, "DROP TABLE test_sec_client_tx")
	_, err = db.ExecContext(ctx, "CREATE TABLE test_sec_client_tx (id INT NOT NULL PRIMARY KEY, title VARCHAR(50))")
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE test_sec_client_tx")
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	clientCtx := WithClientInfo(ctx, ClientInfo{
		ApplicationName: "audit_tx_app",
		WorkstationName: "node_sec_01",
		UserID:          "auditor",
	})

	_, err = tx.ExecContext(clientCtx, "INSERT INTO test_sec_client_tx (id, title) VALUES (?, ?)", 999, "TxClientInfoEntry")
	if err != nil {
		t.Fatalf("tx.ExecContext with WithClientInfo failed: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("tx.Rollback failed: %v", err)
	}

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_sec_client_tx WHERE id = 999").Scan(&count)
	if err != nil {
		t.Fatalf("QueryRowContext failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("ACID FAILURE: Expected 0 rows after Rollback with WithClientInfo, got %d", count)
	}
}

// 15. SEC-15: Null Byte Sanitization in Administrative Commands
func TestSecurity_NullByteSanitization_AdminCmd(t *testing.T) {
	cmd := "RUNSTATS ON TABLE USERS\x00; DROP TABLE USERS;"
	_, err := ExecAdminCmd(context.Background(), nil, cmd)
	if err == nil {
		t.Fatal("expected error executing ExecAdminCmd on nil *sql.DB, got nil")
	}

	// Verify that a command consisting solely of null bytes and whitespace is rejected as empty
	onlyNulls := "\x00\x00   \x00"
	_, err = ExecAdminCmd(context.Background(), nil, onlyNulls)
	if err == nil {
		t.Fatal("expected error for command containing only null bytes, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected 'cannot be empty' error, got: %v", err)
	}
}

// 16. SEC-16: Context Metadata (WithUser & WithClientInfo) Enforcement in Prepared Statements
func TestSecurity_PreparedStatement_ContextMetadata(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointEXCSQLSET, drda.CodePointRDBCMM, drda.CodePointEXCSAT:
			return writeEXCSATRDOrOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://BASEUSER:password@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
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
	err = driverConn.Raw(func(raw any) error {
		if c, ok := raw.(*Conn); ok {
			conn = c
		}
		return nil
	})
	if err != nil || conn == nil {
		t.Fatalf("failed to extract *Conn: %v", err)
	}

	stmt := newStmt(conn, "SELECT 1 FROM SYSIBM.SYSDUMMY1", nil, nil)

	maliciousCtx := WithUser(context.Background(), "ADMIN; DROP TABLE USERS; --")

	// 1. Stmt.ExecContext must enforce user validation / switching
	_, err = stmt.ExecContext(maliciousCtx, nil)
	if err == nil {
		t.Fatal("expected error executing Stmt.ExecContext with malicious WithUser, got nil (authorization bypass!)")
	}

	// 2. Stmt.QueryContext must enforce user validation / switching
	_, err = stmt.QueryContext(maliciousCtx, nil)
	if err == nil {
		t.Fatal("expected error executing Stmt.QueryContext with malicious WithUser, got nil (authorization bypass!)")
	}

	// 3. Conn.PrepareContext must enforce user validation / switching
	_, err = conn.PrepareContext(maliciousCtx, "SELECT 1 FROM SYSIBM.SYSDUMMY1")
	if err == nil {
		t.Fatal("expected error executing Conn.PrepareContext with malicious WithUser, got nil (authorization bypass!)")
	}

	// 4. Verify client info register propagation
	info := ClientInfo{
		ApplicationName: "audit_app_stmt",
		WorkstationName: "node_sec_stmt",
		UserID:          "auditor_stmt",
	}
	clientCtx := WithClientInfo(context.Background(), info)
	if err := conn.applyContextMetadata(clientCtx); err != nil {
		t.Fatalf("unexpected error applying client info: %v", err)
	}
}

// 17. SEC-17: Password Redaction in DSN Parsing Errors
func TestSecurity_PasswordRedaction_DSNError(t *testing.T) {
	secretPwd := "SuperSecretP@ssw0rd!123"
	malformedURLDSN := fmt.Sprintf("db2://admin:%s@localhost:50000/db%%xx", secretPwd)

	_, err := ParseDSN(malformedURLDSN)
	if err == nil {
		t.Fatal("expected error parsing malformed DSN URL, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, secretPwd) {
		t.Fatalf("ParseDSN leaked plaintext password in error message: %s", errMsg)
	}
	if !strings.Contains(errMsg, "******") {
		t.Fatalf("ParseDSN error message did not contain redacted '******': %s", errMsg)
	}
}

// 18. SEC-18: ResetSession Optimization and Error Handling
func TestSecurity_ResetSession_OptimizationAndErrorHandling(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointEXCSQLSET, drda.CodePointRDBCMM, drda.CodePointRDBRLLBCK:
			return writeSQLCARDOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://BASEUSER:password@"+srv.addr+"/TESTDB?ssl=false&client_applname=base_app&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	driverConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer driverConn.Close()

	var conn *Conn
	err = driverConn.Raw(func(raw any) error {
		if c, ok := raw.(*Conn); ok {
			conn = c
		}
		return nil
	})
	if err != nil || conn == nil {
		t.Fatalf("failed to extract *Conn: %v", err)
	}

	// 1. Unmodified connection: ResetSession is zero-network-cost in-memory check
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatalf("ResetSession on unmodified connection failed: %v", err)
	}

	if !conn.session.ClientInfoMatches("base_app", "", "", "", "") {
		t.Fatalf("expected ClientInfoMatches base_app")
	}

	// 2. Modify registers during pool usage
	conn.session.SetAutoCommit(false)
	if err := conn.SetClientInfo(ctx, ClientInfo{ApplicationName: "temp_app"}); err != nil {
		t.Fatalf("SetClientInfo failed: %v", err)
	}

	// Verify registers were updated
	if conn.session.ClientInfoMatches("base_app", "", "", "", "") {
		t.Fatalf("expected ClientInfo to differ after SetClientInfo")
	}

	// ResetSession restores registers back to base config ("base_app")
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}

	if !conn.session.ClientInfoMatches("base_app", "", "", "", "") {
		t.Fatalf("expected ClientInfo to be restored to base_app after ResetSession")
	}

	if !conn.session.AutoCommit() {
		t.Fatal("expected AutoCommit=true after ResetSession")
	}

	// 3. Closed / Broken connection returns driver.ErrBadConn
	_ = conn.Close()
	if err := conn.ResetSession(ctx); err != driver.ErrBadConn {
		t.Fatalf("expected driver.ErrBadConn on closed connection, got: %v", err)
	}
}

// 19. SEC-19: Multi-Group ParseQRYDSC Descriptor Parsing for >84 Query Columns
func TestSecurity_ParseQRYDSC_MultiGroupOver84Columns(t *testing.T) {
	numCols := 100 // Exceeds single FDODSC group limit of 84
	var payload []byte

	for i := 0; i < numCols; {
		chunkSize := numCols - i
		if chunkSize > 84 {
			chunkSize = 84
		}
		groupLen := (1 + chunkSize) * 3
		payload = append(payload, byte(groupLen), 0x76, 0xD0)
		for j := 0; j < chunkSize; j++ {
			colIdx := i + j
			payload = append(payload, byte(colIdx%256), 0x01, 0x02)
		}
		i += chunkSize
	}

	fields, err := drda.ParseQRYDSC(payload)
	if err != nil {
		t.Fatalf("ParseQRYDSC failed for %d columns across multiple descriptor groups: %v", numCols, err)
	}

	if len(fields) != numCols {
		t.Fatalf("expected %d field descriptors extracted, got %d (columns truncated!)", numCols, len(fields))
	}

	for i, f := range fields {
		if f.Type != byte(i%256) {
			t.Errorf("column %d: expected type %d, got %d", i+1, i%256, f.Type)
		}
	}
}

// 20. SEC-20: CRLF & Control Character Sanitization in Client Audit Special Registers
func TestSecurity_ClientInfo_CRLF_Sanitization(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointEXCSQLSET, drda.CodePointRDBCMM, drda.CodePointEXCSAT:
			return writeEXCSATRDOrOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://BASEUSER:password@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
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
	err = driverConn.Raw(func(raw any) error {
		if c, ok := raw.(*Conn); ok {
			conn = c
		}
		return nil
	})
	if err != nil || conn == nil {
		t.Fatalf("failed to extract *Conn: %v", err)
	}

	info := ClientInfo{
		ApplicationName:  "audit\r\napp",
		WorkstationName:  "node\nhost",
		UserID:           "user\rname",
		Accounting:       "acct\r\n'dept",
		CorrelationToken: "corr\n'token",
	}

	if err := conn.SetClientInfo(ctx, info); err != nil {
		t.Fatalf("SetClientInfo failed: %v", err)
	}

	if !conn.session.ClientInfoMatches("auditapp", "nodehost", "username", "acct'dept", "corr'token") {
		t.Fatal("ClientInfo failed to strip CRLF control characters and preserve unescaped string in memory")
	}
}

// 22. SEC-22: Context Metadata (WithUser) Username Identifier Validation
func TestSecurity_WithUser_Validation(t *testing.T) {
	conn := &Conn{
		session: &drda.Session{},
	}

	invalidUsers := []string{
		"ADMIN; DROP TABLE USERS; --",
		"USER' OR '1'='1",
		"ROOT\x00ADMIN",
		"USER WITH SPACES",
		"USER-MINUS",
	}

	for _, user := range invalidUsers {
		t.Run("InvalidUser_"+user, func(t *testing.T) {
			ctx := WithUser(context.Background(), user)
			err := conn.applyContextMetadata(ctx)
			if err == nil {
				t.Fatalf("expected error applying context metadata with invalid WithUser %q, got nil", user)
			}
			if !strings.Contains(err.Error(), "invalid context user identifier") {
				t.Fatalf("expected invalid identifier error message, got: %v", err)
			}
		})
	}

	validUsers := []string{
		"VALID_USER",
		"USER123",
		"APP_USER#1",
		"DB2ADMIN$",
	}

	for _, user := range validUsers {
		t.Run("ValidUser_"+user, func(t *testing.T) {
			ctx := WithUser(context.Background(), user)
			// SwitchUser on dummy session will fail, but applyContextMetadata must pass validation
			err := conn.applyContextMetadata(ctx)
			if err != nil && strings.Contains(err.Error(), "invalid context user identifier") {
				t.Fatalf("unexpected validation error for valid context user %q: %v", user, err)
			}
		})
	}
}

// 21. SEC-21: sql.Out nil destination pointer dereference protection
func TestSecurity_SqlOut_NilDestinationPanicProtection(t *testing.T) {
	conn := &Conn{
		session: &drda.Session{},
	}
	stmt := newStmt(conn, "CALL MY_PROC(?)", nil, []drda.ColumnDescription{
		{SQLType: uint16(codec.SQLTypeInteger)},
	})

	testCases := []struct {
		name string
		arg  driver.NamedValue
	}{
		{
			name: "sql.Out with untyped nil Dest",
			arg:  driver.NamedValue{Ordinal: 1, Value: sql.Out{Dest: nil}},
		},
		{
			name: "sql.Out with typed nil pointer Dest",
			arg:  driver.NamedValue{Ordinal: 1, Value: sql.Out{Dest: (*int)(nil)}},
		},
		{
			name: "*sql.Out with untyped nil Dest",
			arg:  driver.NamedValue{Ordinal: 1, Value: &sql.Out{Dest: nil}},
		},
		{
			name: "*sql.Out with typed nil pointer Dest",
			arg:  driver.NamedValue{Ordinal: 1, Value: &sql.Out{Dest: (*int32)(nil)}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("execContextLocked panicked on %s: %v", tc.name, r)
				}
			}()

			_, err := stmt.ExecContext(context.Background(), []driver.NamedValue{tc.arg})
			// Connection/session is dummy, so we expect a non-panic execution error or nil if handled
			_ = err
		})
	}
}

// 23. SEC-23: ParseSQLDTARD extended length DDM parameter header parsing and bounds validation
func TestSecurity_ParseSQLDTARD_ExtendedLengthAndBounds(t *testing.T) {
	// Construct an FDODSC sub-object with normal length (CodePoint 0x0010)
	fdodscPayload := []byte{
		0x06, 0x76, 0xD0, // descriptor group header
		0x02, 0x00, 0x04, // Integer (0x02), PS=(0,4)
		0x06, 0x71, 0xE4, 0xD0, 0x00, 0x01, // end descriptor
	}
	var fdodscObj []byte
	fdodscObj = binary.BigEndian.AppendUint16(fdodscObj, uint16(4+len(fdodscPayload)))
	fdodscObj = binary.BigEndian.AppendUint16(fdodscObj, 0x0010)
	fdodscObj = append(fdodscObj, fdodscPayload...)

	// Construct an FDODTA sub-object with extended length header 0x8008 (CodePoint 0x147A)
	// Payload: 0xFF 0x00 header + 4-byte int32 value (42)
	fdodtaPayload := []byte{0xFF, 0x00, 0x00, 0x00, 0x00, 0x2A}
	var fdodtaObj []byte
	fdodtaObj = binary.BigEndian.AppendUint16(fdodtaObj, 0x8008) // Extended length flag (0x8000 + 8)
	fdodtaObj = binary.BigEndian.AppendUint16(fdodtaObj, 0x147A) // FDODTA CodePoint
	fdodtaObj = binary.BigEndian.AppendUint32(fdodtaObj, uint32(len(fdodtaPayload)))
	fdodtaObj = append(fdodtaObj, fdodtaPayload...)

	sqldtardPayload := append(fdodscObj, fdodtaObj...)

	res, err := codec.ParseSQLDTARD(sqldtardPayload, binary.BigEndian)
	if err != nil {
		t.Fatalf("ParseSQLDTARD failed for extended length FDODTA parameter: %v", err)
	}

	if len(res) != 1 {
		t.Fatalf("expected 1 output parameter parsed, got %d", len(res))
	}

	if val, ok := res[0].(int64); !ok || val != 42 {
		t.Fatalf("expected output parameter value 42, got %v (%T)", res[0], res[0])
	}

	// Test malformed payload bounds
	malformed := []byte{0x80, 0x08, 0x14, 0x7A, 0x00, 0x00, 0x01, 0xFF} // claims 0x1FF bytes payload but data truncated
	_, err = codec.ParseSQLDTARD(malformed, binary.BigEndian)
	if err == nil {
		t.Fatalf("expected error for malformed extended length payload, got nil")
	}
}

// 24. SEC-11: BeginTx context metadata & broken check, plus ResetSession rollback order
func TestSecurity_BeginTx_ContextMetadataAndBrokenCheck_And_ResetSessionRollbackOrder(t *testing.T) {
	var mu sync.Mutex
	var cpOrder []drda.CodePoint
	dropOnIMM := false
	dropOnRLLBCK := false

	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		mu.Lock()
		cpOrder = append(cpOrder, cp)
		shouldDrop := (dropOnIMM && cp == drda.CodePointEXCSQLIMM) || (dropOnRLLBCK && cp == drda.CodePointRDBRLLBCK)
		mu.Unlock()

		if shouldDrop {
			return false
		}
		switch cp {
		case drda.CodePointEXCSQLSET, drda.CodePointEXCSQLIMM, drda.CodePointRDBCMM, drda.CodePointRDBRLLBCK:
			return writeSQLCARDOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://BASEUSER:password@"+srv.addr+"/TESTDB?ssl=false&client_applname=base_app&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sqlConn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer sqlConn.Close()

	var conn *Conn
	if err := sqlConn.Raw(func(raw any) error {
		conn = raw.(*Conn)
		return nil
	}); err != nil || conn == nil {
		t.Fatalf("failed to extract *Conn: %v", err)
	}

	// 1. BeginTx rejects invalid WithUser before sending SET CURRENT ISOLATION
	mu.Lock()
	cpOrder = nil
	mu.Unlock()

	maliciousCtx := WithUser(ctx, "ADMIN; DROP TABLE USERS; --")
	if _, err := conn.BeginTx(maliciousCtx, driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelSerializable)}); err == nil {
		t.Fatal("expected BeginTx with invalid WithUser to fail before starting tx")
	}
	mu.Lock()
	if len(cpOrder) != 0 {
		t.Fatalf("expected 0 wire requests after rejected WithUser in BeginTx, got %v", cpOrder)
	}
	mu.Unlock()

	// 2. BeginTx applies WithUser (SwitchUser) + WithClientInfo BEFORE SET CURRENT ISOLATION
	metaCtx := WithUser(WithClientInfo(ctx, ClientInfo{ApplicationName: "tx_audit_app"}), "ALTUSER")
	tx, err := conn.BeginTx(metaCtx, driver.TxOptions{Isolation: driver.IsolationLevel(sql.LevelSerializable)})
	if err != nil {
		t.Fatalf("BeginTx with valid metadata failed: %v", err)
	}
	if tx == nil {
		t.Fatal("expected non-nil tx")
	}

	mu.Lock()
	gotBegin := append([]drda.CodePoint(nil), cpOrder...)
	cpOrder = nil
	mu.Unlock()

	// SwitchUser emits EXCSQLIMM (SET SESSION AUTHORIZATION), then SetClientInfo (EXCSQLSET), then SET CURRENT ISOLATION (EXCSQLIMM)
	if len(gotBegin) < 3 || gotBegin[0] != drda.CodePointEXCSQLIMM || gotBegin[1] != drda.CodePointEXCSQLSET || gotBegin[2] != drda.CodePointEXCSQLIMM {
		t.Fatalf("expected BeginTx order [EXCSQLIMM(SwitchUser), EXCSQLSET(ClientInfo), EXCSQLIMM(Isolation)], got %v", gotBegin)
	}
	if conn.CurrentUser() != "ALTUSER" {
		t.Fatalf("expected CurrentUser 'ALTUSER' after BeginTx(WithUser), got %q", conn.CurrentUser())
	}

	// 3. ResetSession while !AutoCommit && hasCustomIso && dirty ClientInfo MUST emit RDBRLLBCK FIRST
	if err := conn.ResetSession(ctx); err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}
	mu.Lock()
	gotReset := append([]drda.CodePoint(nil), cpOrder...)
	cpOrder = nil
	mu.Unlock()

	if len(gotReset) < 2 || gotReset[0] != drda.CodePointRDBRLLBCK {
		t.Fatalf("expected ResetSession to emit RDBRLLBCK first, got %v", gotReset)
	}
	if !conn.session.AutoCommit() {
		t.Fatal("expected AutoCommit=true after ResetSession")
	}

	// 4. BeginTx on a broken session returns driver.ErrBadConn
	mu.Lock()
	dropOnIMM = true
	mu.Unlock()
	_, _ = conn.ExecContext(ctx, "SET CURRENT DEGREE = '1'", nil)
	if !conn.session.Broken() {
		t.Fatal("expected session to be marked broken after dropped EXCSQLIMM")
	}
	if _, err := conn.BeginTx(ctx, driver.TxOptions{}); err != driver.ErrBadConn {
		t.Fatalf("expected driver.ErrBadConn from BeginTx on broken session, got %v", err)
	}

	// 5. ResetSession returns driver.ErrBadConn when RDBRLLBCK fails
	sqlConn2, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("second db.Conn failed: %v", err)
	}
	defer sqlConn2.Close()

	var conn2 *Conn
	if err := sqlConn2.Raw(func(raw any) error {
		conn2 = raw.(*Conn)
		return nil
	}); err != nil || conn2 == nil {
		t.Fatalf("failed to extract second *Conn: %v", err)
	}
	conn2.session.SetAutoCommit(false)

	mu.Lock()
	dropOnIMM = false
	dropOnRLLBCK = true
	mu.Unlock()

	if err := conn2.ResetSession(ctx); err != driver.ErrBadConn {
		t.Fatalf("expected driver.ErrBadConn from ResetSession when Rollback fails, got %v", err)
	}
}

var _ driver.Stmt = (*Stmt)(nil)
