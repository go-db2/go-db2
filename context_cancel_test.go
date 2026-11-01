package db2

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/go-db2/go-db2/network"
)

// A query the server never answers must return once its context is done,
// with the context's error, and the connection must not be reused.
func TestQueryReturnsWhenContextIsDone(t *testing.T) {
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })

	srv := startMockServer(t, func(c net.Conn, cp network.CodePoint) bool {
		switch cp {
		case network.CodePointPRPSQLSTT:
			<-hang // never reply, like a long-running statement
			return false
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

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := db.QueryContext(ctx, "SELECT 1 FROM SYSIBM.SYSDUMMY1")
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("QueryContext did not return after its context deadline")
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		t.Fatalf("ping after an interrupted query should run on a fresh connection, got: %v", err)
	}
	if got := srv.accepts.Load(); got != 2 {
		t.Fatalf("expected the interrupted connection to be replaced (2 connections), got %d", got)
	}
}

// Connecting to a server that accepts the socket but never answers the
// handshake must also honour the context.
func TestConnectReturnsWhenContextIsDone(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			defer c.Close() // hold the socket open, never reply
		}
	}()

	db, err := sql.Open("db2", "db2://db2inst1:password@"+listener.Addr().String()+"/SAMPLE?ssl=false&allow_plaintext_auth=true")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- db.PingContext(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connecting did not return after the context deadline")
	}
}
