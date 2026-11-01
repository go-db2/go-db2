package db2

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-db2/go-db2/network"
)

// AC-1: Downgrade protection (SECMEC 9 -> 3)
func TestSecurity_DowngradeRefusal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	var secchkReceived atomic.Bool

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read EXCSAT + ACCSEC
		_, _ = readRequestChain(conn)

		// Respond ACCSECRD offering downgraded SECMEC 3
		accsecRdObj := network.PackDDMObject(network.CodePointACCSECRD, network.PackUint16(network.CodePointSECMEC, network.SecMecUSRIDPWD))
		hdr := network.BuildDSSHeader(len(accsecRdObj), network.DSSTypeReply, false, false, false, 1)
		_, _ = conn.Write(append(hdr, accsecRdObj...))

		// Check if client sends SECCHK (it must NOT)
		cp, ok := readRequestChain(conn)
		if ok && cp == network.CodePointSECCHK {
			secchkReceived.Store(true)
		}
	}()

	dsn := fmt.Sprintf("db2://db2inst1:SecretPass123@%s/SAMPLE?ssl=false&allow_plaintext_auth=true&security_mechanism=dh_encrypted_password", listener.Addr().String())
	cfg, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}

	session := network.NewSession(cfg.ToSessionConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = session.Connect(ctx)
	if err == nil {
		t.Fatal("expected Connect to fail due to SECMEC downgrade, got nil")
	}

	if !strings.Contains(err.Error(), "refusing downgrade") {
		t.Fatalf("expected error containing 'refusing downgrade', got: %v", err)
	}

	if secchkReceived.Load() {
		t.Fatal("client sent SECCHK after server requested downgrade!")
	}
}

// AC-2: SECTKN validation (< 20 bytes for SECMEC 9)
func TestSecurity_SECTKNValidation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read EXCSAT + ACCSEC
		_, _ = readRequestChain(conn)

		// Respond ACCSECRD with SECMEC 9 but empty/short SECTKN (e.g. 5 bytes instead of >= 20)
		body := network.PackUint16(network.CodePointSECMEC, network.SecMecEUSRIDPWD)
		body = append(body, network.PackBytes(network.CodePointSECTKN, []byte{0x01, 0x02, 0x03, 0x04, 0x05})...)
		accsecRdObj := network.PackDDMObject(network.CodePointACCSECRD, body)
		hdr := network.BuildDSSHeader(len(accsecRdObj), network.DSSTypeReply, false, false, false, 1)
		_, _ = conn.Write(append(hdr, accsecRdObj...))
	}()

	dsn := fmt.Sprintf("db2://db2inst1:SecretPass123@%s/SAMPLE?ssl=false&allow_plaintext_auth=true&security_mechanism=dh_encrypted_password", listener.Addr().String())
	cfg, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}

	session := network.NewSession(cfg.ToSessionConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = session.Connect(ctx)
	if err == nil {
		t.Fatal("expected Connect to fail due to short SECTKN, got nil")
	}

	if !strings.Contains(err.Error(), "invalid server security token") {
		t.Fatalf("expected error containing 'invalid server security token', got: %v", err)
	}
}

// AC-3: Plaintext without TLS block
func TestSecurity_PlaintextWithoutTLS_Blocked(t *testing.T) {
	// 1. Blocked via Session.Connect directly
	session := network.NewSession(network.SessionConfig{
		Host:               "127.0.0.1",
		Port:               50000,
		Database:           "SAMPLE",
		User:               "alice",
		Password:           "secret",
		UseSSL:             false,
		AllowPlaintextAuth: false,
	})
	ctx := context.Background()
	err := session.Connect(ctx)
	if err == nil {
		t.Fatal("expected Connect to fail for plaintext auth over unencrypted connection, got nil")
	}
	if !strings.Contains(err.Error(), "plaintext authentication over unencrypted connection is disabled by default") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 2. Allowed when AllowPlaintextAuth is true
	sessionAllowed := network.NewSession(network.SessionConfig{
		Host:               "127.0.0.1",
		Port:               1, // Non-listening port, just to verify it passes auth check and fails on dial
		Database:           "SAMPLE",
		User:               "alice",
		Password:           "secret",
		UseSSL:             false,
		AllowPlaintextAuth: true,
	})
	errAllowed := sessionAllowed.Connect(ctx)
	// Should fail with network error, not plaintext auth error
	if strings.Contains(errAllowed.Error(), "plaintext authentication") {
		t.Fatalf("expected network dial error, but got plaintext auth error: %v", errAllowed)
	}
}

// AC-4: Kerberos disabled
func TestSecurity_KerberosDisabled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read EXCSAT + ACCSEC
		_, _ = readRequestChain(conn)

		// Respond ACCSECRD with SECMEC 7
		accsecRdObj := network.PackDDMObject(network.CodePointACCSECRD, network.PackUint16(network.CodePointSECMEC, 7))
		hdr := network.BuildDSSHeader(len(accsecRdObj), network.DSSTypeReply, false, false, false, 1)
		_, _ = conn.Write(append(hdr, accsecRdObj...))
	}()

	cfg := network.SessionConfig{
		Host:               "127.0.0.1",
		Port:               listener.Addr().(*net.TCPAddr).Port,
		Database:           "SAMPLE",
		User:               "alice",
		SecurityMechanism:  7,
		AllowPlaintextAuth: true,
		Timeout:            2 * time.Second,
	}

	session := network.NewSession(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = session.Connect(ctx)
	if err == nil {
		t.Fatal("expected Connect to fail for Kerberos SECMEC 7, got nil")
	}

	if !strings.Contains(err.Error(), "Kerberos security mechanism (SECMEC 7/11) is disabled") {
		t.Fatalf("expected error containing Kerberos disabled message, got: %v", err)
	}
}

// AC-7: HandshakeContext cancellation in TLS
func TestSecurity_TLSHandshakeContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	// Server accepts TCP but halts without completing TLS handshake
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(2 * time.Second)
	}()

	cfg := network.SessionConfig{
		Host:      "127.0.0.1",
		Port:      listener.Addr().(*net.TCPAddr).Port,
		Database:  "SAMPLE",
		User:      "alice",
		Password:  "secret",
		UseSSL:    true,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
		Timeout:   10 * time.Second,
	}

	session := network.NewSession(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = session.Connect(ctx)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected Connect to fail on canceled context, got nil")
	}

	if duration > 1*time.Second {
		t.Fatalf("TLS HandshakeContext did not honor context cancellation promptly: took %v", duration)
	}
}

// AC-8: Ping idempotency in transaction (no COMMIT issued, EXCSAT used)
func TestSecurity_PingIdempotenceInTransaction(t *testing.T) {
	var mu sync.Mutex
	var receivedCodepoints []network.CodePoint

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		if !mockHandshake(conn) {
			return
		}

		for {
			cp, ok := readRequestChain(conn)
			if !ok {
				return
			}
			mu.Lock()
			receivedCodepoints = append(receivedCodepoints, cp)
			mu.Unlock()

			switch cp {
			case network.CodePointEXCSAT:
				_ = writeEXCSATRD(conn)
			case network.CodePointRDBCMM:
				_ = writeSQLCARDOK(conn)
			default:
				_ = writeSQLCARDOK(conn)
			}
		}
	}()

	dsn := fmt.Sprintf("db2://db2inst1:password@%s/SAMPLE?ssl=false&allow_plaintext_auth=true", listener.Addr().String())
	db, err := sql.Open("db2", dsn)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("db.Conn failed: %v", err)
	}
	defer conn.Close()

	// Start explicit transaction
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx failed: %v", err)
	}

	// Ping while inside transaction
	if err := conn.PingContext(ctx); err != nil {
		t.Fatalf("conn.PingContext inside tx failed: %v", err)
	}

	// Verify that ping issued EXCSAT and NOT RDBCMM
	mu.Lock()
	var hasPingEXCSAT bool
	for _, cp := range receivedCodepoints {
		if cp == network.CodePointEXCSAT {
			hasPingEXCSAT = true
		}
		if cp == network.CodePointRDBCMM {
			mu.Unlock()
			t.Fatalf("Ping inside transaction issued RDBCMM (COMMIT)! Received: 0x%04X", cp)
		}
	}
	mu.Unlock()

	if !hasPingEXCSAT {
		t.Fatal("Ping did not issue EXCSAT")
	}

	// Now explicitly commit
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx.Commit failed: %v", err)
	}

	// Now verify RDBCMM was sent for the actual Commit
	mu.Lock()
	var hasCommit bool
	for _, cp := range receivedCodepoints {
		if cp == network.CodePointRDBCMM {
			hasCommit = true
		}
	}
	mu.Unlock()
	if !hasCommit {
		t.Fatal("tx.Commit did not issue RDBCMM")
	}
}
