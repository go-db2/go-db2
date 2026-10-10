package network

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-db2/go-db2/converters"
)

func TestSessionMockHandshake(t *testing.T) {
	// Start a local mock Db2 TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Run mock server in goroutine
	serverErrChan := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErrChan <- err
			return
		}
		defer conn.Close()

		// 1. Read EXCSAT + ACCSEC from client
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				serverErrChan <- err
				return
			}
			if cp == CodePointACCSEC || !hdr.Chained {
				break
			}
		}

		// 2. Respond with ACCSECRD (SECMEC = 3)
		accsecRdBody := PackUint16(CodePointSECMEC, SecMecUSRIDPWD)
		accsecRdObj := PackDDMObject(CodePointACCSECRD, accsecRdBody)
		dssHdr := BuildDSSHeader(len(accsecRdObj), DSSTypeReply, false, false, false, 1)
		if _, err := conn.Write(append(dssHdr, accsecRdObj...)); err != nil {
			serverErrChan <- err
			return
		}

		// 3. Read SECCHK + ACCRDB from client
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				serverErrChan <- err
				return
			}
			if cp == CodePointACCRDB || !hdr.Chained {
				break
			}
		}

		// 4. Respond with SECCHKRM (SECCHKCD = 0) and ACCRDBRM
		secchkRmBody := PackBytes(CodePointSECCHKCD, []byte{0x00})
		secchkRmObj := PackDDMObject(CodePointSECCHKRM, secchkRmBody)
		accrdbRmObj := PackDDMObject(CodePointACCRDBRM, nil)

		// Send chained SECCHKRM -> ACCRDBRM
		hdr1 := BuildDSSHeader(len(secchkRmObj), DSSTypeReply, true, false, false, 1)
		hdr2 := BuildDSSHeader(len(accrdbRmObj), DSSTypeReply, false, false, false, 1)

		var respBuf bytes.Buffer
		respBuf.Write(hdr1)
		respBuf.Write(secchkRmObj)
		respBuf.Write(hdr2)
		respBuf.Write(accrdbRmObj)

		if _, err := conn.Write(respBuf.Bytes()); err != nil {
			serverErrChan <- err
			return
		}

		// 5. Read post-connect setup packets from client
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				serverErrChan <- err
				return
			}
			if cp == CodePointRDBCMM || !hdr.Chained {
				break
			}
		}

		// Respond with dummy SQLCARD
		sqlcardPayload := make([]byte, 20)
		sqlcardPayload[0] = 0x00                              // Valid SQLCARD
		binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0) // SQLCODE = 0
		copy(sqlcardPayload[5:10], "00000")                   // SQLSTATE = 00000
		sqlcardObj := PackDDMObject(CodePointSQLCARD, sqlcardPayload)
		sqlcardHdr := BuildDSSHeader(len(sqlcardObj), DSSTypeReply, false, false, false, 1)

		if _, err := conn.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
			serverErrChan <- err
			return
		}

		// 6. Handle Ping request (RDBCMM)
		_, cp, _, _, err := ReadDSS(conn)
		if err != nil {
			serverErrChan <- err
			return
		}
		if cp == CodePointRDBCMM {
			if _, err := conn.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
				serverErrChan <- err
				return
			}
		}

		serverErrChan <- nil
	}()

	// Run client session
	session := NewSession(SessionConfig{
		Host:     "127.0.0.1",
		Port:     port,
		Database: "SAMPLE",
		User:     "db2inst1",
		Password: "password",
		Timeout:  2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := session.Connect(ctx); err != nil {
		t.Fatalf("session.Connect() failed: %v", err)
	}

	if err := session.Ping(ctx); err != nil {
		t.Fatalf("session.Ping() failed: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session.Close() failed: %v", err)
	}

	if srvErr := <-serverErrChan; srvErr != nil {
		t.Fatalf("server encountered error: %v", srvErr)
	}
}

func TestSessionConnect_SetClientInfoErrorClosesSession(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		// 1. Read EXCSAT + ACCSEC from client
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				_ = conn.Close()
				return
			}
			if cp == CodePointACCSEC || !hdr.Chained {
				break
			}
		}

		// 2. Respond with ACCSECRD
		accsecRdBody := PackUint16(CodePointSECMEC, SecMecUSRIDPWD)
		accsecRdObj := PackDDMObject(CodePointACCSECRD, accsecRdBody)
		dssHdr := BuildDSSHeader(len(accsecRdObj), DSSTypeReply, false, false, false, 1)
		_, _ = conn.Write(append(dssHdr, accsecRdObj...))

		// 3. Read SECCHK + ACCRDB from client
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				_ = conn.Close()
				return
			}
			if cp == CodePointACCRDB || !hdr.Chained {
				break
			}
		}

		// 4. Respond with SECCHKRM and ACCRDBRM
		secchkRmBody := PackBytes(CodePointSECCHKCD, []byte{0x00})
		secchkRmObj := PackDDMObject(CodePointSECCHKRM, secchkRmBody)
		accrdbRmObj := PackDDMObject(CodePointACCRDBRM, nil)

		hdr1 := BuildDSSHeader(len(secchkRmObj), DSSTypeReply, true, false, false, 1)
		hdr2 := BuildDSSHeader(len(accrdbRmObj), DSSTypeReply, false, false, false, 1)

		var respBuf bytes.Buffer
		respBuf.Write(hdr1)
		respBuf.Write(secchkRmObj)
		respBuf.Write(hdr2)
		respBuf.Write(accrdbRmObj)
		_, _ = conn.Write(respBuf.Bytes())

		// 5. Read post-connect setup (EXCSAT -> EXCSQLSET -> SQLSTT -> RDBCMM)
		for {
			_, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				_ = conn.Close()
				return
			}
			if cp == CodePointRDBCMM {
				break
			}
		}

		sqlcardPayload := make([]byte, 20)
		sqlcardPayload[0] = 0x00
		binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0)
		copy(sqlcardPayload[5:10], "00000")
		sqlcardObj := PackDDMObject(CodePointSQLCARD, sqlcardPayload)
		sqlcardHdr := BuildDSSHeader(len(sqlcardObj), DSSTypeReply, false, false, false, 1)
		_, _ = conn.Write(append(sqlcardHdr, sqlcardObj...))

		// 6. Read SetClientInfo request and then close connection to cause SetClientInfo failure
		_, _, _, _, _ = ReadDSS(conn)
		_ = conn.Close()
	}()

	session := NewSession(SessionConfig{
		Host:           "127.0.0.1",
		Port:           port,
		Database:       "SAMPLE",
		User:           "db2inst1",
		Password:       "password",
		ClientApplName: "failing_appl_name",
		Timeout:        2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = session.Connect(ctx)
	if err == nil {
		t.Fatalf("expected Connect to fail when SetClientInfo fails, got nil")
	}

	if !strings.Contains(err.Error(), "failed to apply client info") {
		t.Fatalf("expected error message containing 'failed to apply client info', got: %v", err)
	}

	// Verify session was closed
	session.mu.Lock()
	closed := session.closed
	session.mu.Unlock()

	if !closed {
		t.Fatalf("expected session to be closed after SetClientInfo error in Connect")
	}
}

func TestPackSQLINTR(t *testing.T) {
	pkgid := "SYSSH200"
	pkgcnstkn := "SYSLVL01"
	pkgsn := uint16(4)
	database := "TESTDB"

	pkt := PackSQLINTR(pkgid, pkgcnstkn, pkgsn, database)
	if len(pkt) < 4 {
		t.Fatalf("SQLINTR packet too short: %d", len(pkt))
	}

	codePoint := binary.BigEndian.Uint16(pkt[2:4])
	if codePoint != uint16(CodePointSQLINTR) {
		t.Fatalf("expected codepoint 0x%04X, got 0x%04X", CodePointSQLINTR, codePoint)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ALICE", `"ALICE"`},
		{"tenant_alice", `"tenant_alice"`},
		{"user#1", `"user#1"`},
		{`user"name`, `"user""name"`},
	}

	for _, tt := range tests {
		got := quoteIdentifier(tt.input)
		if got != tt.expected {
			t.Errorf("quoteIdentifier(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestEscapeSingleQuotes_ControlCharactersAndBackslashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Single quote doubling", "User'sApp", "User''sApp"},
		{"Multiple single quotes", "a'b'c'd", "a''b''c''d"},
		{"Null byte removal", "User\x00App", "UserApp"},
		{"Carriage return removal", "User\rApp", "UserApp"},
		{"Line feed removal", "User\nApp", "UserApp"},
		{"Combined CRLF and quotes", "User\r\n's\x00App", "User''sApp"},
		{"Backslash preservation", "C:\\Program Files\\App", "C:\\Program Files\\App"},
		{"Backslash with single quotes", "C:\\User's\\App", "C:\\User''s\\App"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSingleQuotes(tt.input)
			if got != tt.expected {
				t.Errorf("escapeSingleQuotes(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEscapeSingleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"myapp", "myapp"},
		{"my'app", "my''app"},
		{"my\x00app", "myapp"},
		{"my\x00'app", "my''app"},
		{"my\r\napp", "myapp"},
		{"my\n'app\r", "my''app"},
	}

	for _, tt := range tests {
		got := escapeSingleQuotes(tt.input)
		if got != tt.expected {
			t.Errorf("escapeSingleQuotes(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSwitchUser_QuotedIdentifierSQL(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	executedSQLs := make(chan string, 10)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. Complete initial handshake & setup
		for {
			_, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				return
			}
			if cp == CodePointACCSEC {
				accsecRdBody := PackUint16(CodePointSECMEC, SecMecUSRIDPWD)
				accsecRdObj := PackDDMObject(CodePointACCSECRD, accsecRdBody)
				dssHdr := BuildDSSHeader(len(accsecRdObj), DSSTypeReply, false, false, false, 1)
				_, _ = conn.Write(append(dssHdr, accsecRdObj...))
			} else if cp == CodePointACCRDB {
				secchkRmBody := PackBytes(CodePointSECCHKCD, []byte{0x00})
				secchkRmObj := PackDDMObject(CodePointSECCHKRM, secchkRmBody)
				accrdbRmObj := PackDDMObject(CodePointACCRDBRM, nil)

				hdr1 := BuildDSSHeader(len(secchkRmObj), DSSTypeReply, true, false, false, 1)
				hdr2 := BuildDSSHeader(len(accrdbRmObj), DSSTypeReply, false, false, false, 1)

				var respBuf bytes.Buffer
				respBuf.Write(hdr1)
				respBuf.Write(secchkRmObj)
				respBuf.Write(hdr2)
				respBuf.Write(accrdbRmObj)
				_, _ = conn.Write(respBuf.Bytes())
			} else if cp == CodePointRDBCMM {
				sqlcardPayload := make([]byte, 20)
				sqlcardPayload[0] = 0x00
				binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0)
				copy(sqlcardPayload[5:10], "00000")
				sqlcardObj := PackDDMObject(CodePointSQLCARD, sqlcardPayload)
				sqlcardHdr := BuildDSSHeader(len(sqlcardObj), DSSTypeReply, false, false, false, 1)
				_, _ = conn.Write(append(sqlcardHdr, sqlcardObj...))
				break
			}
		}

		// 2. Listen for SwitchUser SQL commands
		for {
			hdr, cp, data, _, err := ReadDSS(conn)
			if err != nil {
				return
			}
			if cp == CodePointSQLSTT {
				if len(data) >= 5 && data[0] == 0x00 {
					sqlLen := binary.BigEndian.Uint32(data[1:5])
					if uint32(len(data)) >= 5+sqlLen {
						sqlText := string(data[5 : 5+sqlLen])
						executedSQLs <- sqlText

						sqlcode := int32(0)
						if strings.HasPrefix(sqlText, "SET SESSION AUTHORIZATION") {
							sqlcode = -1
						}

						sqlcardPayload := make([]byte, 20)
						sqlcardPayload[0] = 0x00
						binary.LittleEndian.PutUint32(sqlcardPayload[1:5], uint32(sqlcode))
						copy(sqlcardPayload[5:10], "00000")
						sqlcardObj := PackDDMObject(CodePointSQLCARD, sqlcardPayload)
						sqlcardHdr := BuildDSSHeader(len(sqlcardObj), DSSTypeReply, false, false, false, hdr.CorrelationID)
						_, _ = conn.Write(append(sqlcardHdr, sqlcardObj...))
					}
				}
			}
		}
	}()

	session := NewSession(SessionConfig{
		Host:     "127.0.0.1",
		Port:     port,
		Database: "SAMPLE",
		User:     "db2inst1",
		Password: "password",
		Timeout:  2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := session.Connect(ctx); err != nil {
		t.Fatalf("session.Connect() failed: %v", err)
	}
	defer session.Close()

	_ = session.SwitchUser(ctx, "tenant_alice")

	select {
	case sql1 := <-executedSQLs:
		expected1 := `SET SESSION AUTHORIZATION = "tenant_alice"`
		if sql1 != expected1 {
			t.Errorf("expected SET SESSION AUTHORIZATION SQL %q, got %q", expected1, sql1)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for SET SESSION AUTHORIZATION statement")
	}

	select {
	case sql2 := <-executedSQLs:
		expected2 := `SET SESSION_USER = "tenant_alice"`
		if sql2 != expected2 {
			t.Errorf("expected SET SESSION_USER SQL %q, got %q", expected2, sql2)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for SET SESSION_USER statement")
	}
}

func TestIsLOBType(t *testing.T) {
	tests := []struct {
		name     string
		typeCode uint8
		expected bool
	}{
		// LOB Locators
		{"DRDATypeLOBLOC", converters.DRDATypeLOBLOC, true},
		{"DRDATypeNLOBLOC", converters.DRDATypeNLOBLOC, true},
		{"DRDATypeCLOBLOC", converters.DRDATypeCLOBLOC, true},
		{"DRDATypeNCLOBLOC", converters.DRDATypeNCLOBLOC, true},
		{"DRDATypeDBCSCLOBLOC", converters.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", converters.DRDATypeNDBCSCLOBLOC, true},

		// LOB Bytes & CSBCS
		{"DRDATypeLOBBytes", converters.DRDATypeLOBBytes, true},
		{"DRDATypeNLOBBytes", converters.DRDATypeNLOBBytes, true},
		{"DRDATypeLOBCSBCS", converters.DRDATypeLOBCSBCS, true},
		{"DRDATypeNLOBCSBCS", converters.DRDATypeNLOBCSBCS, true},

		// XML Types
		{"DRDATypeXML", converters.DRDATypeXML, true},
		{"DRDATypeNXML", converters.DRDATypeNXML, true},

		// Raw Hex Codes supported in isLOBType
		{"Raw Hex 0x10", 0x10, true},
		{"Raw Hex 0x11", 0x11, true},
		{"Raw Hex 0xCD", 0xCD, true},
		{"Raw Hex 0xF4", 0xF4, true},
		{"Raw Hex 0xF5", 0xF5, true},
		{"Raw Hex 0xF6", 0xF6, true},
		{"Raw Hex 0xF7", 0xF7, true},
		{"Raw Hex 0xF8", 0xF8, true},
		{"Raw Hex 0xF9", 0xF9, true},

		// Non-LOB Scalar Types (Negative Cases)
		{"Integer", converters.DRDATypeInteger, false},
		{"Nullable Integer", converters.DRDATypeNInteger, false},
		{"SmallInt", converters.DRDATypeSmall, false},
		{"VarChar", converters.DRDATypeVarChar, false},
		{"Char", converters.DRDATypeChar, false},
		{"Date", converters.DRDATypeDate, false},
		{"Time", converters.DRDATypeTime, false},
		{"Timestamp", converters.DRDATypeTimestamp, false},
		{"Boolean", converters.DRDATypeBoolean, false},
		{"DecFloat", converters.DRDATypeDecFloat, false},

		// Unassigned/Arbitrary Raw Hex Codes (Negative Cases)
		{"Unassigned 0x00", 0x00, false},
		{"Unassigned 0x01", 0x01, false},
		{"Unassigned 0x12", 0x12, false},
		{"Unassigned 0xCC", 0xCC, false},
		{"Unassigned 0xFA", 0xFA, false},
		{"Unassigned 0xFF", 0xFF, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLOBType(tt.typeCode)
			if got != tt.expected {
				t.Errorf("isLOBType(0x%02X) = %v, want %v", tt.typeCode, got, tt.expected)
			}
		})
	}
}

func TestIsCLOBType(t *testing.T) {
	tests := []struct {
		name     string
		typeCode uint8
		expected bool
	}{
		// Positive CLOB types
		{"DRDATypeCLOBLOC", converters.DRDATypeCLOBLOC, true},
		{"DRDATypeNCLOBLOC", converters.DRDATypeNCLOBLOC, true},
		{"DRDATypeDBCSCLOBLOC", converters.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", converters.DRDATypeNDBCSCLOBLOC, true},
		{"DRDATypeLOBCSBCS", converters.DRDATypeLOBCSBCS, true},
		{"DRDATypeNLOBCSBCS", converters.DRDATypeNLOBCSBCS, true},
		{"DRDATypeXML", converters.DRDATypeXML, true},
		{"DRDATypeNXML", converters.DRDATypeNXML, true},

		// Raw Hex Codes supported in isCLOBType
		{"Raw Hex 0xCD", 0xCD, true},
		{"Raw Hex 0xF6", 0xF6, true},
		{"Raw Hex 0xF7", 0xF7, true},
		{"Raw Hex 0xF8", 0xF8, true},
		{"Raw Hex 0xF9", 0xF9, true},

		// Non-CLOB Types (Negative Cases)
		{"BLOB Locator", converters.DRDATypeLOBLOC, false},
		{"Nullable BLOB Locator", converters.DRDATypeNLOBLOC, false},
		{"LOB Bytes", converters.DRDATypeLOBBytes, false},
		{"Nullable LOB Bytes", converters.DRDATypeNLOBBytes, false},
		{"Raw Hex 0x10", 0x10, false},
		{"Raw Hex 0x11", 0x11, false},
		{"Raw Hex 0xF4", 0xF4, false},
		{"Raw Hex 0xF5", 0xF5, false},
		{"Integer", converters.DRDATypeInteger, false},
		{"VarChar", converters.DRDATypeVarChar, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCLOBType(tt.typeCode)
			if got != tt.expected {
				t.Errorf("isCLOBType(0x%02X) = %v, want %v", tt.typeCode, got, tt.expected)
			}
		})
	}
}

func TestIsDBCLOBType(t *testing.T) {
	tests := []struct {
		name     string
		typeCode uint8
		expected bool
	}{
		// Positive DBCLOB types
		{"DRDATypeDBCSCLOBLOC", converters.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", converters.DRDATypeNDBCSCLOBLOC, true},

		// Raw Hex Codes supported in isDBCLOBType
		{"Raw Hex 0xCD", 0xCD, true},
		{"Raw Hex 0xF8", 0xF8, true},
		{"Raw Hex 0xF9", 0xF9, true},

		// Non-DBCLOB Types (Negative Cases)
		{"CLOB Locator", converters.DRDATypeCLOBLOC, false},
		{"Nullable CLOB Locator", converters.DRDATypeNCLOBLOC, false},
		{"BLOB Locator", converters.DRDATypeLOBLOC, false},
		{"LOB Bytes", converters.DRDATypeLOBBytes, false},
		{"LOBCSBCS", converters.DRDATypeLOBCSBCS, false},
		{"XML", converters.DRDATypeXML, false},
		{"Raw Hex 0xF6", 0xF6, false},
		{"Raw Hex 0xF7", 0xF7, false},
		{"Integer", converters.DRDATypeInteger, false},
		{"VarChar", converters.DRDATypeVarChar, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDBCLOBType(tt.typeCode)
			if got != tt.expected {
				t.Errorf("isDBCLOBType(0x%02X) = %v, want %v", tt.typeCode, got, tt.expected)
			}
		})
	}
}

func BenchmarkQuoteIdentifier_NoQuotes(b *testing.B) {
	name := "TEST_DATABASE_IDENTIFIER"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = quoteIdentifier(name)
	}
}

func TestSessionConfig_StringAndGoString(t *testing.T) {
	cfg := SessionConfig{
		Host:     "db2.example.com",
		Port:     50000,
		Database: "TESTDB",
		User:     "db2admin",
		Password: "super_secret_password",
		UseSSL:   true,
		Timeout:  10 * time.Second,
	}

	expected := `SessionConfig{Host:db2.example.com, Port:50000, Database:TESTDB, User:db2admin, Password:"******", UseSSL:true, Timeout:10s}`

	if str := cfg.String(); str != expected {
		t.Errorf("SessionConfig.String() = %q, want %q", str, expected)
	}
	if goStr := cfg.GoString(); goStr != expected {
		t.Errorf("SessionConfig.GoString() = %q, want %q", goStr, expected)
	}

	if strings.Contains(cfg.String(), "super_secret_password") {
		t.Errorf("SessionConfig.String() leaked sensitive password!")
	}
}

func TestSessionConfig_SanitizeLogInjection(t *testing.T) {
	cfg := SessionConfig{
		Host:     "db2.example.com\r\n[SECURITY] Fake log entry",
		Port:     50000,
		Database: "TESTDB\nINJECTED",
		User:     "db2admin\x00user",
		Password: "super_secret_password",
		UseSSL:   true,
		Timeout:  10 * time.Second,
	}

	expected := `SessionConfig{Host:db2.example.com[SECURITY] Fake log entry, Port:50000, Database:TESTDBINJECTED, User:db2adminuser, Password:"******", UseSSL:true, Timeout:10s}`

	if str := cfg.String(); str != expected {
		t.Errorf("SessionConfig.String() = %q, want %q", str, expected)
	}
	if goStr := cfg.GoString(); goStr != expected {
		t.Errorf("SessionConfig.GoString() = %q, want %q", goStr, expected)
	}

	if strings.Contains(cfg.String(), "\r") || strings.Contains(cfg.String(), "\n") || strings.Contains(cfg.String(), "\x00") {
		t.Errorf("SessionConfig.String() contains unsanitized control characters!")
	}
}

func BenchmarkQuoteIdentifier_WithQuotes(b *testing.B) {
	name := `TEST_"DATABASE"_IDENTIFIER`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = quoteIdentifier(name)
	}
}
