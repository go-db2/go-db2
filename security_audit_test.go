package db2

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-db2/go-db2/converters"
	"github.com/go-db2/go-db2/network"
	"github.com/go-db2/go-db2/network/security"
	"github.com/go-db2/go-db2/types"
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

	session := &network.Session{}

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

	sessCfg := cfg.ToSessionConfig()
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

	colTypes := make([]types.SQLType, numParams)
	colLens := make([]int64, numParams)
	precs := make([]int, numParams)
	scales := make([]int, numParams)
	args := make([]any, numParams)

	for i := 0; i < numParams; i++ {
		colTypes[i] = types.SQLTypeInteger
		colLens[i] = 4
		precs[i] = 0
		scales[i] = 0
		args[i] = int32(i + 1)
	}

	sqldta, err := converters.BuildSQLDTA(colTypes, colLens, precs, scales, args, binary.BigEndian)
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
	paramCols := []network.ColumnDescription{
		{SQLType: uint16(types.SQLTypeInteger)},
		{SQLType: uint16(types.SQLTypeVarChar)},
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

	pMinusOne := new(big.Int).Sub(security.DHSecmec9Prime, big.NewInt(1))
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
			_, err := security.CalculateSessionKey(tc.key, clientPriv)
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
		_, err := converters.FDODTA(types.SQLTypeDate, 10, 0, 0, invalid, binary.BigEndian)
		if err == nil {
			t.Fatalf("expected error for invalid date %q, got nil", invalid)
		}
	}
}

// 8. SEC-10 & SEC-11: Mutex Concurrency on Stmt and Result
func TestSecurity_Stmt_And_Result_Concurrency(t *testing.T) {
	conn := &Conn{}
	stmt := NewStmt(conn, "SELECT 1 FROM SYSIBM.SYSDUMMY1 WHERE ID = ?", nil, []network.ColumnDescription{
		{SQLType: uint16(types.SQLTypeInteger)},
	})

	var wg sync.WaitGroup
	numWorkers := 30

	// Test Close and state checking concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = stmt.NumInput()
			res := NewResult(int64(id), int64(id*10))
			_, _ = res.RowsAffected()
		}(i)
	}

	wg.Wait()
	_ = stmt.Close()
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
	_, _, _, _, err := network.ReadDSS(reader)
	if err == nil {
		t.Fatal("expected error when objLen > dssLen - 6, got nil")
	}
}

// 10. SEC-03: DecodeField panic protection for truncated metadata (len(ps) < 2)
func TestSecurity_DecodeField_TruncatedMetadataPanicProtection(t *testing.T) {
	testTypes := []uint8{
		converters.DRDATypeChar,
		converters.DRDATypeGraphic,
		converters.DRDATypeVarChar,
		converters.DRDATypeDate,
		converters.DRDATypeTime,
		converters.DRDATypeTimestamp,
		converters.DRDATypeDecimal,
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
				_, err := converters.DecodeField(dt, ps, dummyData, binary.LittleEndian)
				if err == nil {
					t.Fatalf("expected error for truncated ps with len %d, got nil", len(ps))
				}
			})
		}
	}
}

// 11. SEC-07: PackDDMObject protection against uint16 overflow
func TestSecurity_PackDDMObject_LargePayloadProtection(t *testing.T) {
	largeBody := make([]byte, 70000)
	obj := network.PackDDMObject(network.CodePointSQLSTT, largeBody)
	if len(obj) > 65535 {
		t.Fatalf("PackDDMObject exceeded uint16 max: %d bytes", len(obj))
	}
	totalLen := binary.BigEndian.Uint16(obj[0:2])
	if totalLen != 65535 {
		t.Fatalf("expected totalLen=65535, got %d", totalLen)
	}
}

// 12. SEC-01 & SEC-05: Session AutoCommit state and Tx concurrency
func TestSecurity_Session_AutoCommitSwitching(t *testing.T) {
	sess := network.NewSession(network.SessionConfig{Database: "TESTDB"})
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
	_, err = db.ExecContext(ctx, "CREATE TABLE test_sec_tx_iso (id INT PRIMARY KEY, name VARCHAR(50))")
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

var _ driver.Stmt = (*Stmt)(nil)

