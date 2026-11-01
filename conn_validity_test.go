package db2

import (
	"context"
	"database/sql"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/go-db2/go-db2/network"
)

// A reply the client cannot frame leaves unread bytes on the socket. The
// connection must not go back into the pool: the next request on it would
// read those bytes as the start of its own reply.
func TestConnectionIsDiscardedAfterUnreadableReply(t *testing.T) {
	srv := startMockServer(t, func(c net.Conn, cp network.CodePoint) bool {
		switch cp {
		case network.CodePointPRPSQLSTT:
			// A DSS frame whose DDM object claims more bytes than the frame
			// holds, followed by the rest of the frame.
			frame := make([]byte, 20)
			binary.BigEndian.PutUint16(frame[0:2], 20)
			frame[2] = network.DSSMagic
			frame[3] = network.DSSTypeReply
			binary.BigEndian.PutUint16(frame[4:6], 1)
			binary.BigEndian.PutUint16(frame[6:8], 0x0100)
			binary.BigEndian.PutUint16(frame[8:10], uint16(network.CodePointQRYDTA))
			_, err := c.Write(frame)
			return err == nil
		case network.CodePointEXCSAT, network.CodePointRDBCMM:
			return writeEXCSATRDOrOK(c)
		}
		return false
	})

	db, err := sql.Open("db2", "db2://db2inst1:password@"+srv.addr+"/SAMPLE?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.QueryContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1"); err == nil {
		t.Fatal("expected the unreadable reply to fail the query")
	}

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping after a failed reply should run on a fresh connection, got: %v", err)
	}
	if got := srv.accepts.Load(); got != 2 {
		t.Fatalf("expected the broken connection to be replaced (2 connections), got %d", got)
	}
}
