package network

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
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
		sqlcardPayload[0] = 0x00 // Valid SQLCARD
		binary.LittleEndian.PutUint32(sqlcardPayload[1:5], 0) // SQLCODE = 0
		copy(sqlcardPayload[5:10], "00000")                  // SQLSTATE = 00000
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
