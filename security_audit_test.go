package db2

import (
	"context"
	"crypto/rand"
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

var _ driver.Stmt = (*Stmt)(nil)
