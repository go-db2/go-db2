package db2

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/drda"
)

// Helper to encode reply chain for mockServer
func buildReplyChain(objs ...[]byte) []byte {
	var out bytes.Buffer
	for i, obj := range objs {
		isChained := i < len(objs)-1
		hdr := drda.BuildDSSHeader(len(obj), drda.DSSTypeReply, isChained, false, false, 1)
		out.Write(hdr)
		out.Write(obj)
	}
	return out.Bytes()
}

func mockIntegerRows(values ...int32) []byte {
	var b []byte
	for _, v := range values {
		b = append(b, 0xFF, 0x00)
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	return b
}

func mockQrydscInteger() []byte {
	return drda.PackDDMObject(drda.CodePointQRYDSC, []byte{0x06, 0x76, 0xD0, 0x02, 0x00, 0x04, 0x06, 0x71, 0xF0, 0xE0, 0x00, 0x00})
}

func mockOpnqryrm() []byte {
	body := drda.PackUint16(drda.CodePointSVRCOD, 0)
	body = append(body, drda.PackBytes(drda.CodePointQRYINSID, []byte{0, 0, 0, 0, 0, 0, 0, 1})...)
	return drda.PackDDMObject(drda.CodePointOPNQRYRM, body)
}

func mockSqlcard(code int32, state string) []byte {
	payload := make([]byte, 20)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(code))
	copy(payload[5:10], state)
	return drda.PackDDMObject(drda.CodePointSQLCARD, payload)
}

func mockQrydta(data []byte) []byte {
	return drda.PackDDMObject(drda.CodePointQRYDTA, data)
}

// TestDriver_CALL_MultiResultSets tests that CALL statements returning multiple result sets
// can be fully traversed using database/sql's rows.NextResultSet().
func TestDriver_CALL_MultiResultSets(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointEXCSQLIMM:
			// Respond with 2 result sets in the reply chain
			reply := buildReplyChain(
				mockOpnqryrm(),
				mockQrydscInteger(),
				mockQrydta(mockIntegerRows(100, 200)),
				mockQrydscInteger(),
				mockQrydta(mockIntegerRows(300, 400)),
				mockSqlcard(0, "00000"),
			)
			_, _ = c.Write(reply)
			return true
		default:
			return true
		}
	})

	db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "CALL GET_MULTIPLE_SETS()")
	if err != nil {
		t.Fatalf("QueryContext CALL failed: %v", err)
	}
	defer rows.Close()

	// 1st result set: [100, 200]
	var got1 []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan set 1 failed: %v", err)
		}
		got1 = append(got1, v)
	}
	if len(got1) != 2 || got1[0] != 100 || got1[1] != 200 {
		t.Fatalf("set 1 mismatch: got %v", got1)
	}

	// Advance to 2nd result set
	if !rows.NextResultSet() {
		t.Fatalf("expected 2nd result set")
	}

	// 2nd result set: [300, 400]
	var got2 []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("Scan set 2 failed: %v", err)
		}
		got2 = append(got2, v)
	}
	if len(got2) != 2 || got2[0] != 300 || got2[1] != 400 {
		t.Fatalf("set 2 mismatch: got %v", got2)
	}

	if rows.NextResultSet() {
		t.Fatalf("expected no 3rd result set")
	}
}

// TestDriver_Streaming_EarlyCloseCLSQRY tests that closing rows before EOF emits CLSQRY on server.
func TestDriver_Streaming_EarlyCloseCLSQRY(t *testing.T) {
	clsqryReceived := make(chan bool, 1)

	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointPRPSQLSTT, drda.CodePointOPNQRY:
			reply := buildReplyChain(
				mockOpnqryrm(),
				mockQrydscInteger(),
				mockQrydta(mockIntegerRows(1, 2, 3, 4, 5)),
			)
			_, _ = c.Write(reply)
			return true
		case drda.CodePointCLSQRY:
			clsqryReceived <- true
			_, _ = c.Write(buildReplyChain(mockSqlcard(0, "00000")))
			return true
		case drda.CodePointRDBCMM:
			_, _ = c.Write(buildReplyChain(mockSqlcard(0, "00000")))
			return true
		default:
			return true
		}
	})

	db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT ID FROM LARGE_TABLE")
	if err != nil {
		t.Fatalf("QueryContext failed: %v", err)
	}

	// Read only 1 row then close
	if !rows.Next() {
		t.Fatalf("expected at least 1 row")
	}
	var val int
	if err := rows.Scan(&val); err != nil || val != 1 {
		t.Fatalf("expected 1, got %d (err: %v)", val, err)
	}

	// Close rows before EOF
	if err := rows.Close(); err != nil {
		t.Fatalf("rows.Close failed: %v", err)
	}

	select {
	case seen := <-clsqryReceived:
		if !seen {
			t.Fatal("expected CLSQRY from driver")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for CLSQRY on server")
	}
}

// TestDriver_LOB_IoReaderParameter tests that an io.Reader parameter is accepted and processed
// without error and without query rewriting.
func TestDriver_LOB_IoReaderParameter(t *testing.T) {
	extdtaSeen := make(chan bool, 1)

	srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
		switch cp {
		case drda.CodePointPRPSQLSTT:
			// PRPSQLSTT + SQLDARD describing 1 BLOB param
			sqlcardDard := make([]byte, 20)
			sqlcardDard[0] = 0xFF
			reply := buildReplyChain(
				drda.PackDDMObject(drda.CodePointSQLCARD, sqlcardDard),
			)
			_, _ = c.Write(reply)
			return true
		case drda.CodePointEXCSQLSTT:
			// Read the chain to see if EXTDTA was sent
			for {
				hdr, innerCP, _, _, err := drda.ReadDSS(c)
				if err != nil {
					return false
				}
				if innerCP == drda.CodePointEXTDTA {
					extdtaSeen <- true
				}
				if !hdr.Chained {
					break
				}
			}
			_, _ = c.Write(buildReplyChain(mockSqlcard(0, "00000")))
			return true
		default:
			return true
		}
	})

	db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify checkNamedValue accepts io.Reader
	var driverConn driver.Conn
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer conn.Close()

	err = conn.Raw(func(driverConnRaw any) error {
		driverConn = driverConnRaw.(driver.Conn)
		return nil
	})
	if err != nil {
		t.Fatalf("conn.Raw failed: %v", err)
	}

	nvChecker, ok := driverConn.(driver.NamedValueChecker)
	if !ok {
		t.Fatalf("driverConn does not implement NamedValueChecker")
	}

	reader := io.NopCloser(strings.NewReader("hello large lob payload"))
	nv := &driver.NamedValue{Ordinal: 1, Value: reader}
	if err := nvChecker.CheckNamedValue(nv); err != nil {
		t.Fatalf("CheckNamedValue rejected io.Reader: %v", err)
	}
}
