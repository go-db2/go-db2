package drda

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/codec"
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

		// 6. Handle Ping request (EXCSAT / RDBCMM)
		_, cp, _, _, err := ReadDSS(conn)
		if err != nil {
			serverErrChan <- err
			return
		}
		switch cp {
		case CodePointEXCSAT:
			excsatRdObj := PackDDMObject(CodePointEXCSATRD, nil)
			excsatRdHdr := BuildDSSHeader(len(excsatRdObj), DSSTypeReply, false, false, false, 1)
			if _, err := conn.Write(append(excsatRdHdr, excsatRdObj...)); err != nil {
				serverErrChan <- err
				return
			}
		case CodePointRDBCMM:
			if _, err := conn.Write(append(sqlcardHdr, sqlcardObj...)); err != nil {
				serverErrChan <- err
				return
			}
		}

		serverErrChan <- nil
	}()

	// Run client session
	session := NewSession(SessionConfig{
		Host:               "127.0.0.1",
		Port:               port,
		Database:           "SAMPLE",
		User:               "db2inst1",
		Password:           "password",
		AllowPlaintextAuth: true,
		Timeout:            2 * time.Second,
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

func TestSession_Connect_SetClientInfoError(t *testing.T) {
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
		Host:               "127.0.0.1",
		Port:               port,
		Database:           "SAMPLE",
		User:               "db2inst1",
		Password:           "password",
		AllowPlaintextAuth: true,
		ClientApplName:     "failing_appl_name",
		Timeout:            2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = session.Connect(ctx)
	if err == nil {
		t.Fatalf("expected Connect to fail when SetClientInfo fails, got nil")
	}

	if !strings.Contains(err.Error(), "failed to apply client info (session closed)") {
		t.Fatalf("expected error message containing 'failed to apply client info (session closed)', got: %v", err)
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
		Host:               "127.0.0.1",
		Port:               port,
		Database:           "SAMPLE",
		User:               "db2inst1",
		Password:           "password",
		AllowPlaintextAuth: true,
		Timeout:            2 * time.Second,
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
		{"DRDATypeLOBLOC", codec.DRDATypeLOBLOC, true},
		{"DRDATypeNLOBLOC", codec.DRDATypeNLOBLOC, true},
		{"DRDATypeCLOBLOC", codec.DRDATypeCLOBLOC, true},
		{"DRDATypeNCLOBLOC", codec.DRDATypeNCLOBLOC, true},
		{"DRDATypeDBCSCLOBLOC", codec.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", codec.DRDATypeNDBCSCLOBLOC, true},

		// LOB Bytes & CSBCS
		{"DRDATypeLOBBytes", codec.DRDATypeLOBBytes, true},
		{"DRDATypeNLOBBytes", codec.DRDATypeNLOBBytes, true},
		{"DRDATypeLOBCSBCS", codec.DRDATypeLOBCSBCS, true},
		{"DRDATypeNLOBCSBCS", codec.DRDATypeNLOBCSBCS, true},

		// XML Types
		{"DRDATypeXML", codec.DRDATypeXML, true},
		{"DRDATypeNXML", codec.DRDATypeNXML, true},

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
		{"Integer", codec.DRDATypeInteger, false},
		{"Nullable Integer", codec.DRDATypeNInteger, false},
		{"SmallInt", codec.DRDATypeSmall, false},
		{"VarChar", codec.DRDATypeVarChar, false},
		{"Char", codec.DRDATypeChar, false},
		{"Date", codec.DRDATypeDate, false},
		{"Time", codec.DRDATypeTime, false},
		{"Timestamp", codec.DRDATypeTimestamp, false},
		{"Boolean", codec.DRDATypeBoolean, false},
		{"DecFloat", codec.DRDATypeDecFloat, false},

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
		{"DRDATypeCLOBLOC", codec.DRDATypeCLOBLOC, true},
		{"DRDATypeNCLOBLOC", codec.DRDATypeNCLOBLOC, true},
		{"DRDATypeDBCSCLOBLOC", codec.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", codec.DRDATypeNDBCSCLOBLOC, true},
		{"DRDATypeLOBCSBCS", codec.DRDATypeLOBCSBCS, true},
		{"DRDATypeNLOBCSBCS", codec.DRDATypeNLOBCSBCS, true},
		{"DRDATypeXML", codec.DRDATypeXML, true},
		{"DRDATypeNXML", codec.DRDATypeNXML, true},

		// Raw Hex Codes supported in isCLOBType
		{"Raw Hex 0xCD", 0xCD, true},
		{"Raw Hex 0xF6", 0xF6, true},
		{"Raw Hex 0xF7", 0xF7, true},
		{"Raw Hex 0xF8", 0xF8, true},
		{"Raw Hex 0xF9", 0xF9, true},

		// Non-CLOB Types (Negative Cases)
		{"BLOB Locator", codec.DRDATypeLOBLOC, false},
		{"Nullable BLOB Locator", codec.DRDATypeNLOBLOC, false},
		{"LOB Bytes", codec.DRDATypeLOBBytes, false},
		{"Nullable LOB Bytes", codec.DRDATypeNLOBBytes, false},
		{"Raw Hex 0x10", 0x10, false},
		{"Raw Hex 0x11", 0x11, false},
		{"Raw Hex 0xF4", 0xF4, false},
		{"Raw Hex 0xF5", 0xF5, false},
		{"Integer", codec.DRDATypeInteger, false},
		{"VarChar", codec.DRDATypeVarChar, false},
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
		{"DRDATypeDBCSCLOBLOC", codec.DRDATypeDBCSCLOBLOC, true},
		{"DRDATypeNDBCSCLOBLOC", codec.DRDATypeNDBCSCLOBLOC, true},

		// Raw Hex Codes supported in isDBCLOBType
		{"Raw Hex 0xCD", 0xCD, true},
		{"Raw Hex 0xF8", 0xF8, true},
		{"Raw Hex 0xF9", 0xF9, true},

		// Non-DBCLOB Types (Negative Cases)
		{"CLOB Locator", codec.DRDATypeCLOBLOC, false},
		{"Nullable CLOB Locator", codec.DRDATypeNCLOBLOC, false},
		{"BLOB Locator", codec.DRDATypeLOBLOC, false},
		{"LOB Bytes", codec.DRDATypeLOBBytes, false},
		{"LOBCSBCS", codec.DRDATypeLOBCSBCS, false},
		{"XML", codec.DRDATypeXML, false},
		{"Raw Hex 0xF6", 0xF6, false},
		{"Raw Hex 0xF7", 0xF7, false},
		{"Integer", codec.DRDATypeInteger, false},
		{"VarChar", codec.DRDATypeVarChar, false},
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

func BenchmarkEscapeSingleQuotes_Clean(b *testing.B) {
	val := "my_application_name_123"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = escapeSingleQuotes(val)
	}
}

func BenchmarkEscapeSingleQuotes_WithQuotes(b *testing.B) {
	val := "User's Application Identifier '2026'"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = escapeSingleQuotes(val)
	}
}

func BenchmarkEscapeSingleQuotes_WithControlChars(b *testing.B) {
	val := "User\r\n's\x00Application\n"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = escapeSingleQuotes(val)
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

func TestSession_Handshake_RefuseDowngrade(t *testing.T) {
	testCases := []struct {
		name          string
		clientSecmec  uint16
		password      string
		serverSecmec  uint16
		expectedError string
	}{
		{
			name:          "Refuse downgrade from SECMEC 4 to 3",
			clientSecmec:  4,
			password:      "",
			serverSecmec:  3,
			expectedError: "db2: server rejected security mechanism 4 and offered 3; refusing downgrade",
		},
		{
			name:          "Refuse downgrade from SECMEC 9 to 3",
			clientSecmec:  9,
			password:      "pass",
			serverSecmec:  3,
			expectedError: "db2: server rejected security mechanism 9 and offered 3; refusing downgrade",
		},
		{
			name:          "Refuse unsolicited change from SECMEC 3 to 9",
			clientSecmec:  3,
			password:      "pass",
			serverSecmec:  9,
			expectedError: "db2: server rejected security mechanism 3 and offered 9; refusing downgrade",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("failed to start listener: %v", err)
			}
			defer listener.Close()

			port := listener.Addr().(*net.TCPAddr).Port

			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				for {
					hdr, cp, _, _, err := ReadDSS(conn)
					if err != nil {
						return
					}
					if cp == CodePointACCSEC || !hdr.Chained {
						break
					}
				}

				accsecRdBody := PackUint16(CodePointSECMEC, tc.serverSecmec)
				accsecRdObj := PackDDMObject(CodePointACCSECRD, accsecRdBody)
				dssHdr := BuildDSSHeader(len(accsecRdObj), DSSTypeReply, false, false, false, 1)
				_, _ = conn.Write(append(dssHdr, accsecRdObj...))
			}()

			session := NewSession(SessionConfig{
				Host:               "127.0.0.1",
				Port:               port,
				Database:           "SAMPLE",
				User:               "user",
				Password:           tc.password,
				SecurityMechanism:  tc.clientSecmec,
				AllowPlaintextAuth: true,
				Timeout:            2 * time.Second,
			})

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = session.Connect(ctx)
			if err == nil || !strings.Contains(err.Error(), tc.expectedError) {
				t.Fatalf("expected error containing %q, got: %v", tc.expectedError, err)
			}
		})
	}
}

func TestSession_Handshake_PlaintextBlocked(t *testing.T) {
	// AC-2: Connect must immediately reject plaintext auth without TLS or AllowPlaintextAuth
	session := NewSession(SessionConfig{
		Host:               "127.0.0.1",
		Port:               50000,
		Database:           "SAMPLE",
		User:               "user",
		Password:           "pass",
		SecurityMechanism:  3,
		UseSSL:             false,
		AllowPlaintextAuth: false,
	})

	err := session.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "plaintext authentication over unencrypted connection is disabled by default") {
		t.Fatalf("expected plaintext disabled error, got: %v", err)
	}
}

func TestSession_SwitchUser_PasswordDirectSECCHK(t *testing.T) {
	// AC-3: SwitchUser with password must not execute SQL commands; it must send SECCHK directly
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	var receivedCodepoints []CodePoint
	var receivedSQLStatements []string

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, cp, data, _, err := ReadDSS(conn)
			if err != nil {
				return
			}
			receivedCodepoints = append(receivedCodepoints, cp)
			if cp == CodePointSQLSTT {
				receivedSQLStatements = append(receivedSQLStatements, string(data))
			}
			if cp == CodePointSECCHK {
				secchkRmBody := PackBytes(CodePointSECCHKCD, []byte{0x00})
				secchkRmObj := PackDDMObject(CodePointSECCHKRM, secchkRmBody)
				hdr := BuildDSSHeader(len(secchkRmObj), DSSTypeReply, false, false, false, 1)
				_, _ = conn.Write(append(hdr, secchkRmObj...))
			}
		}
	}()

	clientConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	session := &Session{
		conn:          clientConn,
		secmec:        3,
		encoding:      EncodingCP500,
		correlationID: 1,
		cfg: SessionConfig{
			Database:           "SAMPLE",
			User:               "OLDUSER",
			AllowPlaintextAuth: true,
		},
	}
	defer session.Close()

	err = session.SwitchUser(context.Background(), "NEWUSER", "NEWPASS")
	if err != nil {
		t.Fatalf("SwitchUser failed: %v", err)
	}

	if session.CurrentUser() != "NEWUSER" {
		t.Errorf("CurrentUser = %q, want NEWUSER", session.CurrentUser())
	}

	if len(receivedSQLStatements) > 0 {
		t.Fatalf("expected no SQL statements to be sent when password is provided, got: %v", receivedSQLStatements)
	}

	foundSECCHK := false
	for _, cp := range receivedCodepoints {
		if cp == CodePointSECCHK {
			foundSECCHK = true
			break
		}
	}
	if !foundSECCHK {
		t.Fatalf("expected CodePointSECCHK in received packets")
	}
}

func TestSession_SwitchUser_SECMEC9_TokenHandling(t *testing.T) {
	// AC-4: SwitchUser with SECMEC 9 without valid secTkn must fail immediately
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := &Session{
		conn:   clientConn,
		secmec: 9,
		secTkn: nil,
		cfg: SessionConfig{
			Database: "SAMPLE",
			User:     "OLDUSER",
		},
	}

	err := session.SwitchUser(context.Background(), "NEWUSER", "PASS")
	if err == nil || !strings.Contains(err.Error(), "ACCSEC negotiation token is missing or invalid for SECMEC 9") {
		t.Fatalf("expected missing token error, got: %v", err)
	}
}

func TestSession_SwitchUser_RepliesValidation(t *testing.T) {
	// AC-5: SwitchUser error handling on failure replies
	testCases := []struct {
		name        string
		replyPacket []byte
		expectUser  string
	}{
		{
			name: "RDBAFLRM failure",
			replyPacket: func() []byte {
				obj := PackDDMObject(CodePointRDBAFLRM, nil)
				hdr := BuildDSSHeader(len(obj), DSSTypeReply, false, false, false, 1)
				return append(hdr, obj...)
			}(),
			expectUser: "OLDUSER",
		},
		{
			name: "SECCHKCD error failure",
			replyPacket: func() []byte {
				body := PackBytes(CodePointSECCHKCD, []byte{0x0B}) // Userid/Password invalid
				obj := PackDDMObject(CodePointSECCHKRM, body)
				hdr := BuildDSSHeader(len(obj), DSSTypeReply, false, false, false, 1)
				return append(hdr, obj...)
			}(),
			expectUser: "OLDUSER",
		},
		{
			name: "Missing SECCHKRM reply",
			replyPacket: func() []byte {
				obj := PackDDMObject(CodePointACCRDBRM, nil)
				hdr := BuildDSSHeader(len(obj), DSSTypeReply, false, false, false, 1)
				return append(hdr, obj...)
			}(),
			expectUser: "OLDUSER",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("failed to start listener: %v", err)
			}
			defer listener.Close()

			port := listener.Addr().(*net.TCPAddr).Port

			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				_, _, _, _, _ = ReadDSS(conn)
				_, _ = conn.Write(tc.replyPacket)
			}()

			clientConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				t.Fatalf("failed to dial: %v", err)
			}

			session := &Session{
				conn:          clientConn,
				secmec:        3,
				encoding:      EncodingCP500,
				correlationID: 1,
				cfg: SessionConfig{
					Database:           "SAMPLE",
					User:               "OLDUSER",
					AllowPlaintextAuth: true,
				},
			}
			defer session.Close()

			err = session.SwitchUser(context.Background(), "NEWUSER", "PASS")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if session.CurrentUser() != tc.expectUser {
				t.Fatalf("expected user %q to remain unchanged, got: %q", tc.expectUser, session.CurrentUser())
			}
		})
	}
}

func TestSession_SwitchUser_PlaintextBlocked(t *testing.T) {
	// AC-6: SwitchUser with SECMEC 3 and password must be rejected if unencrypted and !AllowPlaintextAuth
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	session := &Session{
		conn:   clientConn,
		secmec: 3,
		cfg: SessionConfig{
			Database:           "SAMPLE",
			User:               "OLDUSER",
			UseSSL:             false,
			AllowPlaintextAuth: false,
		},
	}

	err := session.SwitchUser(context.Background(), "NEWUSER", "PASS")
	if err == nil || !strings.Contains(err.Error(), "plaintext authentication over unencrypted connection is disabled by default") {
		t.Fatalf("expected plaintext auth error, got: %v", err)
	}
	if session.CurrentUser() != "OLDUSER" {
		t.Fatalf("user was updated despite error: %q", session.CurrentUser())
	}
}

func TestSession_USRIDONL_AndWhitelistValidation(t *testing.T) {
	const secretPass = "SecretPassword123!"

	t.Run("Connect blocks USRIDONL with password before network dial", func(t *testing.T) {
		s := NewSession(SessionConfig{
			Host:              "127.0.0.1",
			Port:              1, // closed port: pre-connect validation must fail before dial
			Database:          "SAMPLE",
			User:              "USER1",
			Password:          secretPass,
			SecurityMechanism: SecMecUSRIDONL,
			Timeout:           time.Second,
		})
		err := s.Connect(context.Background())
		if err == nil || !strings.Contains(err.Error(), "USRIDONL (4) does not allow a password") {
			t.Fatalf("expected USRIDONL password error before dial, got: %v", err)
		}
		if strings.Contains(err.Error(), secretPass) {
			t.Fatalf("error leaked password: %v", err)
		}
	})

	t.Run("Connect blocks unsupported security mechanisms before network dial", func(t *testing.T) {
		for _, secmec := range []uint16{6, 99} {
			s := NewSession(SessionConfig{
				Host:              "127.0.0.1",
				Port:              1,
				Database:          "SAMPLE",
				User:              "USER1",
				Password:          secretPass,
				SecurityMechanism: secmec,
				Timeout:           time.Second,
			})
			err := s.Connect(context.Background())
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("unsupported security mechanism %d", secmec)) {
				t.Fatalf("secmec %d: expected unsupported error before dial, got: %v", secmec, err)
			}
			if strings.Contains(err.Error(), secretPass) {
				t.Fatalf("secmec %d: error leaked password: %v", secmec, err)
			}
		}
	})

	t.Run("SwitchUser blocks USRIDONL with password and unsupported or disabled mechanisms", func(t *testing.T) {
		cases := []struct {
			name       string
			secmec     uint16
			wantErrSub string
		}{
			{
				name:       "USRIDONL (4) with password",
				secmec:     SecMecUSRIDONL,
				wantErrSub: "USRIDONL (4) does not allow a password",
			},
			{
				name:       "unsupported 0 with password",
				secmec:     0,
				wantErrSub: "unsupported security mechanism 0",
			},
			{
				name:       "unsupported 6 with password",
				secmec:     6,
				wantErrSub: "unsupported security mechanism 6",
			},
			{
				name:       "unsupported 99 with password",
				secmec:     99,
				wantErrSub: "unsupported security mechanism 99",
			},
			{
				name:       "disabled Kerberos 7 with password",
				secmec:     7,
				wantErrSub: "Kerberos security mechanism (SECMEC 7/11) is disabled",
			},
			{
				name:       "disabled Kerberos 11 with password",
				secmec:     11,
				wantErrSub: "Kerberos security mechanism (SECMEC 7/11) is disabled",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				clientConn, serverConn := net.Pipe()
				defer clientConn.Close()
				defer serverConn.Close()

				s := &Session{
					conn:     clientConn,
					secmec:   tc.secmec,
					encoding: EncodingCP500,
					cfg: SessionConfig{
						Database:           "SAMPLE",
						User:               "OLDUSER",
						AllowPlaintextAuth: true,
					},
				}

				err := s.SwitchUser(context.Background(), "NEWUSER", secretPass)
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErrSub, err)
				}
				if strings.Contains(err.Error(), secretPass) {
					t.Fatalf("error leaked password: %v", err)
				}
				if s.CurrentUser() != "OLDUSER" {
					t.Fatalf("CurrentUser mutated on error: %q", s.CurrentUser())
				}
			})
		}
	})
}

func TestSession_USRIDONL_OmitsPasswordAndToken_And_SwitchUserSQLFastPath(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	type capturedResult struct {
		sawUSRID     bool
		sawSECMEC    bool
		secMecVal    uint16
		sawPassword  bool
		sawSecTkn    bool
		switchHadSec bool
		err          error
	}
	resCh := make(chan capturedResult, 1)

	go func() {
		var res capturedResult
		conn, err := ln.Accept()
		if err != nil {
			res.err = err
			resCh <- res
			return
		}
		defer conn.Close()

		// 1. Read EXCSAT + ACCSEC
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				res.err = err
				resCh <- res
				return
			}
			if cp == CodePointACCSEC || !hdr.Chained {
				break
			}
		}

		// 2. Reply ACCSECRD (SECMEC = 4)
		accsecRdObj := PackDDMObject(CodePointACCSECRD, PackUint16(CodePointSECMEC, SecMecUSRIDONL))
		dssHdr := BuildDSSHeader(len(accsecRdObj), DSSTypeReply, false, false, false, 1)
		if _, err := conn.Write(append(dssHdr, accsecRdObj...)); err != nil {
			res.err = err
			resCh <- res
			return
		}

		// 3. Read SECCHK + ACCRDB and inspect SECCHK DDM child parameters
		for {
			hdr, cp, data, _, err := ReadDSS(conn)
			if err != nil {
				res.err = err
				resCh <- res
				return
			}
			if cp == CodePointSECCHK {
				for off := 0; off+4 <= len(data); {
					pLen := int(binary.BigEndian.Uint16(data[off : off+2]))
					pCP := CodePoint(binary.BigEndian.Uint16(data[off+2 : off+4]))
					if pLen < 4 || off+pLen > len(data) {
						res.err = fmt.Errorf("invalid DDM param len=%d off=%d total=%d", pLen, off, len(data))
						resCh <- res
						return
					}
					switch pCP {
					case CodePointUSRID:
						res.sawUSRID = true
					case CodePointSECMEC:
						res.sawSECMEC = true
						if pLen >= 6 {
							res.secMecVal = binary.BigEndian.Uint16(data[off+4 : off+6])
						}
					case CodePointPASSWORD:
						res.sawPassword = true
					case CodePointSECTKN:
						res.sawSecTkn = true
					}
					off += pLen
				}
			}
			if cp == CodePointACCRDB || !hdr.Chained {
				break
			}
		}

		// 4. Reply SECCHKRM (SECCHKCD = 0) + ACCRDBRM
		secchkRmObj := PackDDMObject(CodePointSECCHKRM, PackBytes(CodePointSECCHKCD, []byte{0x00}))
		accrdbRmObj := PackDDMObject(CodePointACCRDBRM, nil)
		hdr1 := BuildDSSHeader(len(secchkRmObj), DSSTypeReply, true, false, false, 1)
		hdr2 := BuildDSSHeader(len(accrdbRmObj), DSSTypeReply, false, false, false, 1)
		var resp bytes.Buffer
		resp.Write(hdr1)
		resp.Write(secchkRmObj)
		resp.Write(hdr2)
		resp.Write(accrdbRmObj)
		if _, err := conn.Write(resp.Bytes()); err != nil {
			res.err = err
			resCh <- res
			return
		}

		// 5. Read post-connect setup (RDBCMM / EXCSAT) and reply SQLCARD (0)
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				res.err = err
				resCh <- res
				return
			}
			if cp == CodePointRDBCMM || !hdr.Chained {
				break
			}
		}
		if _, err := conn.Write(replyChain(sqlcard(0, "00000"))); err != nil {
			res.err = err
			resCh <- res
			return
		}

		// 6. Handle SwitchUser (empty password) -> must receive EXCSQLIMM + SQLSTT (not SECCHK)
		for {
			hdr, cp, _, _, err := ReadDSS(conn)
			if err != nil {
				res.err = err
				resCh <- res
				return
			}
			if cp == CodePointSECCHK {
				res.switchHadSec = true
			}
			if !hdr.Chained {
				break
			}
		}
		_, _ = conn.Write(replyChain(sqlcard(0, "00000")))
		resCh <- res
	}()

	addr := ln.Addr().(*net.TCPAddr)
	s := NewSession(SessionConfig{
		Host:               "127.0.0.1",
		Port:               addr.Port,
		Database:           "SAMPLE",
		User:               "OLDUSER",
		Password:           "",
		SecurityMechanism:  SecMecUSRIDONL,
		AllowPlaintextAuth: false, // USRIDONL with no password works without AllowPlaintextAuth
		Timeout:            2 * time.Second,
	})
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect with USRIDONL failed: %v", err)
	}
	defer s.Close()

	if err := s.SwitchUser(context.Background(), "NEWUSER", ""); err != nil {
		t.Fatalf("SwitchUser without password failed: %v", err)
	}
	if s.CurrentUser() != "NEWUSER" {
		t.Fatalf("CurrentUser = %q, want NEWUSER", s.CurrentUser())
	}

	res := <-resCh
	if res.err != nil {
		t.Fatalf("mock server error: %v", res.err)
	}
	if !res.sawUSRID || !res.sawSECMEC || res.secMecVal != SecMecUSRIDONL {
		t.Fatalf("expected USRID=%v SECMEC=%v (val=%d), got mismatch", res.sawUSRID, res.sawSECMEC, res.secMecVal)
	}
	if res.sawPassword || res.sawSecTkn {
		t.Fatalf("USRIDONL SECCHK must omit PASSWORD and SECTKN (sawPassword=%v sawSecTkn=%v)", res.sawPassword, res.sawSecTkn)
	}
	if res.switchHadSec {
		t.Fatalf("SwitchUser without password unexpectedly sent SECCHK instead of SQL fast-path")
	}
}

func TestSession_ReplyChainLimit_ReadReplyChain(t *testing.T) {
	prev := MaxReplyChainSize()
	defer SetMaxReplyChainSize(prev)
	SetMaxReplyChainSize(256)

	s, server := pipeSession(t)
	go func() {
		p1 := PackDDMObject(CodePointSQLCARD, make([]byte, 120))
		p2 := PackDDMObject(CodePointSQLCARD, make([]byte, 120))
		p3 := PackDDMObject(CodePointSQLCARD, make([]byte, 120))
		_, _ = server.Write(replyChain(p1, p2, p3))
	}()

	_, err := s.readReplyChain()
	assertErrReplyChainTooLarge(t, "readReplyChain", err)
	if !s.Broken() {
		t.Fatalf("expected session.Broken() == true after ErrReplyChainTooLarge")
	}
}

func TestSession_ReplyChainLimit_HasOnlyDARD(t *testing.T) {
	prev := MaxReplyChainSize()
	defer SetMaxReplyChainSize(prev)
	SetMaxReplyChainSize(300)

	cases := []struct {
		name string
		call func(s *Session) error
	}{
		{
			name: "ExecWithParams",
			call: func(s *Session) error {
				_, _, err := s.ExecWithParams(context.Background(), ReservedDirectSec, nil, nil)
				return err
			},
		},
		{
			name: "QueryCallDirect",
			call: func(s *Session) error {
				_, err := s.QueryCallDirect(context.Background(), "CALL TEST_PROC()")
				return err
			},
		},
		{
			name: "QueryCallWithParams",
			call: func(s *Session) error {
				_, _, err := s.QueryCallWithParams(context.Background(), ReservedDirectSec, nil, nil)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, server := pipeSession(t)
			go func() {
				for {
					hdr, _, _, _, err := ReadDSS(server)
					if err != nil {
						return
					}
					if !hdr.Chained {
						break
					}
				}
				// First chain: only SQLDARD (180 bytes <= 300), triggers second readReplyChainBudget(180)
				chain1 := replyChain(PackDDMObject(CodePointSQLDARD, make([]byte, 180)))
				if _, err := server.Write(chain1); err != nil {
					return
				}
				// Second chain: SQLCARD (160 bytes) -> combined 340 > 300
				chain2 := replyChain(PackDDMObject(CodePointSQLCARD, make([]byte, 160)))
				_, _ = server.Write(chain2)
			}()

			err := tc.call(s)
			assertErrReplyChainTooLarge(t, tc.name, err)
			if !s.Broken() {
				t.Fatalf("%s: expected session.Broken() == true", tc.name)
			}
		})
	}
}

func TestSession_ReplyChainLimit_FetchQuery(t *testing.T) {
	prev := MaxReplyChainSize()
	defer SetMaxReplyChainSize(prev)
	SetMaxReplyChainSize(450)

	s, server := pipeSession(t)
	cntqrys := make(chan int, 1)

	block := integerRows(1, 2, 3, 4, 5, 6, 7, 8, 9, 10) // 60 bytes per QRYDTA
	continues := make([][][]byte, 0, 8)
	for i := 0; i < 7; i++ {
		continues = append(continues, [][]byte{qrydta(block)})
	}
	continues = append(continues, [][]byte{qrydta(block), endqryrm(), sqlcard(100, "02000")})

	go serveQuery(t, server, queryScript{
		open:      [][]byte{opnqryrm(), qrydscInteger()},
		continues: continues,
	}, cntqrys)

	_, _, err := s.QueryDirect(context.Background(), "SELECT N FROM T")
	assertErrReplyChainTooLarge(t, "fetchQuery", err)
	if !s.Broken() {
		t.Fatalf("expected session.Broken() == true after fetchQuery overflow")
	}
	_ = s.Close()
}

func TestSession_ReplyChainLimit_QueryStream(t *testing.T) {
	prev := MaxReplyChainSize()
	defer SetMaxReplyChainSize(prev)

	t.Run("tailBuf spanning across CNTQRY cycles exceeds limit", func(t *testing.T) {
		SetMaxReplyChainSize(650)

		// 70 non-nullable INT4 columns: 2 row/SQLCA bytes (0xFF, 0x00) + 70*4 = 282 bytes per row
		const numCols = 70
		dscBody := make([]byte, 0, 3+numCols*3+6)
		dscBody = append(dscBody, byte((1+numCols)*3), 0x76, 0xD0)
		for i := 0; i < numCols; i++ {
			dscBody = append(dscBody, 0x02, 0x00, 0x04)
		}
		dscBody = append(dscBody, 0x06, 0x71, 0xF0, 0xE0, 0x00, 0x00)
		qrydsc70 := PackDDMObject(CodePointQRYDSC, dscBody)

		// Initial QRYDTA: 1 complete 282-byte row (starts with 0xFF, 0x00) + 60-byte prefix of row 2 (starts with 0xFF, 0x00)
		openQryDta := make([]byte, 282+60)
		openQryDta[0] = 0xFF
		openQryDta[282] = 0xFF

		s, server := pipeSession(t)
		cntqrys := make(chan int, 1)
		go serveQuery(t, server, queryScript{
			open: [][]byte{opnqryrm(), qrydsc70, qrydta(openQryDta)},
			continues: [][][]byte{
				{qrydta(make([]byte, 200))},                                    // tailBuf grows 60 -> 260 (< 282, still incomplete)
				{qrydta(make([]byte, 400)), endqryrm(), sqlcard(100, "02000")}, // 260 + 400 = 660 > 650
			},
		}, cntqrys)

		_, initialRows, qs, err := s.QueryStreamDirect(context.Background(), "SELECT * FROM WIDE_T")
		if err != nil {
			t.Fatalf("QueryStreamDirect unexpected error: %v", err)
		}
		if len(initialRows) != 1 {
			t.Fatalf("expected 1 initial complete row, got %d", len(initialRows))
		}

		// FetchNextBatch issues CNTQRY #1 (260 <= 650) then loops to CNTQRY #2 (260 + 400 = 660 > 650)
		_, err = qs.FetchNextBatch(context.Background())
		assertErrReplyChainTooLarge(t, "FetchNextBatch", err)
		if !s.Broken() {
			t.Fatalf("expected session.Broken() == true")
		}
		_ = s.Close()
	})

	qrydscXML := PackDDMObject(CodePointQRYDSC, []byte{0x06, 0x76, 0xD0, 0xC6, 0x00, 0x00, 0x06, 0x71, 0xF0, 0xE0, 0x00, 0x00})
	xmlRowDta := []byte{0xFF, 0x00, 0x00, 0x00, 0x00, 0x00}
	smallExt := PackDDMObject(CodePointEXTDTA, []byte{0x00, 'a'})

	t.Run("pendingRows awaiting EXTDTA across CNTQRY cycles exceeds limit", func(t *testing.T) {
		SetMaxReplyChainSize(350)

		s, server := pipeSession(t)
		cntqrys := make(chan int, 1)
		go serveQuery(t, server, queryScript{
			// 1 completed initial row (xmlRowDta + smallExt) + 2 pendingRows awaiting EXTDTA
			open: [][]byte{opnqryrm(), qrydscXML, qrydta(xmlRowDta), smallExt, qrydta(xmlRowDta), qrydta(xmlRowDta)},
			continues: [][][]byte{
				{qrydta(xmlRowDta), qrydta(xmlRowDta)},
				{qrydta(xmlRowDta), qrydta(xmlRowDta)},
			},
		}, cntqrys)

		_, initialRows, qs, err := s.QueryStreamDirect(context.Background(), "SELECT X FROM T")
		if err != nil {
			t.Fatalf("QueryStreamDirect failed: %v", err)
		}
		if len(initialRows) != 1 {
			t.Fatalf("expected 1 initial complete row, got %d", len(initialRows))
		}

		// FetchNextBatch drives CNTQRY #1 (4 pending rows -> 280 <= 350) then CNTQRY #2 (6 pending rows -> 420 > 350)
		_, err = qs.FetchNextBatch(context.Background())
		assertErrReplyChainTooLarge(t, "FetchNextBatch", err)
		if !s.Broken() {
			t.Fatalf("expected session.Broken() == true")
		}
		_ = s.Close()
	})

	t.Run("pendingExt awaiting QRYDTA across CNTQRY cycles exceeds limit", func(t *testing.T) {
		SetMaxReplyChainSize(250)

		extChunk := PackDDMObject(CodePointEXTDTA, make([]byte, 140))
		s, server := pipeSession(t)
		cntqrys := make(chan int, 1)
		go serveQuery(t, server, queryScript{
			// 1 completed initial row so QueryStreamDirect returns qs before CNTQRY
			open: [][]byte{opnqryrm(), qrydscXML, qrydta(xmlRowDta), smallExt},
			continues: [][][]byte{
				{extChunk},
				{extChunk},
			},
		}, cntqrys)

		_, initialRows, qs, err := s.QueryStreamDirect(context.Background(), "SELECT X FROM T")
		if err != nil {
			t.Fatalf("QueryStreamDirect failed: %v", err)
		}
		if len(initialRows) != 1 {
			t.Fatalf("expected 1 initial complete row, got %d", len(initialRows))
		}

		// FetchNextBatch drives CNTQRY #1 (140 <= 250) then CNTQRY #2 (140 + 140 = 280 > 250)
		_, err = qs.FetchNextBatch(context.Background())
		assertErrReplyChainTooLarge(t, "FetchNextBatch", err)
		if !s.Broken() {
			t.Fatalf("expected session.Broken() == true")
		}
		_ = s.Close()
	})
}

func TestSession_ReplyChainLimit_ParseCallResultSets(t *testing.T) {
	prev := MaxReplyChainSize()
	defer SetMaxReplyChainSize(prev)
	SetMaxReplyChainSize(300)

	t.Run("parseCallResultSets enforces cumulative limit across multiple result sets", func(t *testing.T) {
		s := NewSession(SessionConfig{Host: "mock", Database: "TESTDB"})
		qrydscBody := []byte{0x06, 0x76, 0xD0, 0x02, 0x00, 0x04, 0x06, 0x71, 0xF0, 0xE0, 0x00, 0x00}
		rowChunk := integerRows(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20) // 120 bytes

		var replies []ReplyPacket
		for i := 0; i < 3; i++ {
			replies = append(replies,
				ReplyPacket{CodePoint: CodePointQRYDSC, Data: qrydscBody},
				ReplyPacket{CodePoint: CodePointQRYDTA, Data: rowChunk},
			)
		}

		_, _, err := s.parseCallResultSets(replies)
		assertErrReplyChainTooLarge(t, "parseCallResultSets", err)
		if !s.Broken() {
			t.Fatalf("expected session.Broken() == true")
		}
	})

	t.Run("QueryCallDirect fails when multi-result-set chain exceeds limit", func(t *testing.T) {
		s, server := pipeSession(t)
		rowChunk := integerRows(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)
		go func() {
			for {
				hdr, _, _, _, err := ReadDSS(server)
				if err != nil {
					return
				}
				if !hdr.Chained {
					break
				}
			}
			_, _ = server.Write(replyChain(
				opnqryrm(),
				qrydscInteger(), qrydta(rowChunk),
				qrydscInteger(), qrydta(rowChunk),
				qrydscInteger(), qrydta(rowChunk),
				sqlcard(0, "00000"),
			))
		}()

		_, err := s.QueryCallDirect(context.Background(), "CALL MULTI_PROC()")
		assertErrReplyChainTooLarge(t, "QueryCallDirect", err)
		if !s.Broken() {
			t.Fatalf("expected session.Broken() == true")
		}
	})
}

type benchSliceConn struct {
	data    []byte
	off     int
	onWrite func(c *benchSliceConn)
}

func (c *benchSliceConn) Read(p []byte) (int, error) {
	if c.off >= len(c.data) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, c.data[c.off:])
	c.off += n
	return n, nil
}

func (c *benchSliceConn) Write(p []byte) (int, error) {
	if c.onWrite != nil {
		c.onWrite(c)
	}
	return len(p), nil
}

func (c *benchSliceConn) Close() error                       { return nil }
func (c *benchSliceConn) LocalAddr() net.Addr                { return nil }
func (c *benchSliceConn) RemoteAddr() net.Addr               { return nil }
func (c *benchSliceConn) SetDeadline(t time.Time) error      { return nil }
func (c *benchSliceConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *benchSliceConn) SetWriteDeadline(t time.Time) error { return nil }

func BenchmarkReadReplyChain(b *testing.B) {
	chain := replyChain(
		opnqryrm(),
		qrydscInteger(),
		qrydta(integerRows(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)),
		sqlcard(100, "02000"),
	)
	conn := &benchSliceConn{data: chain}
	s := NewSession(SessionConfig{Host: "mock", Database: "TESTDB"})
	s.conn = conn

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.off = 0
		replies, err := s.readReplyChain()
		if err != nil {
			b.Fatalf("readReplyChain failed: %v", err)
		}
		if len(replies) == 0 {
			b.Fatal("empty replies")
		}
	}
}

func BenchmarkQueryStream_FetchNextBatch(b *testing.B) {
	openReply := replyChain(opnqryrm(), qrydscInteger(), qrydta(integerRows(1)))
	contReply := replyChain(qrydta(integerRows(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)))

	conn := &benchSliceConn{data: openReply}
	s := NewSession(SessionConfig{Host: "mock", Database: "TESTDB"})
	s.conn = conn

	ctx := context.Background()
	_, _, qs, err := s.QueryStreamDirect(ctx, "SELECT N FROM T")
	if err != nil {
		b.Fatalf("QueryStreamDirect failed: %v", err)
	}

	conn.data = contReply
	conn.off = len(contReply)
	conn.onWrite = func(c *benchSliceConn) {
		if c.off >= len(c.data) {
			c.off = 0
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch, err := qs.FetchNextBatch(ctx)
		if err != nil {
			b.Fatalf("FetchNextBatch failed: %v", err)
		}
		if len(batch) != 10 {
			b.Fatalf("batch len = %d, want 10", len(batch))
		}
	}
}
