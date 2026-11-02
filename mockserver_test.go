package db2

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync/atomic"
	"testing"

	"github.com/go-db2/go-db2/internal/drda"
)

// mockServer is a minimal DRDA server: it completes the connect handshake and
// then hands every request chain to handle, identified by the codepoint of
// the chain's first command. handle returns false to drop the connection.
type mockServer struct {
	addr    string
	accepts atomic.Int32
}

func startMockServer(t *testing.T, handle func(c net.Conn, cp drda.CodePoint) bool) *mockServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	srv := &mockServer{addr: listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			srv.accepts.Add(1)
			go func(c net.Conn) {
				defer c.Close()
				if !mockHandshake(c) {
					return
				}
				for {
					cp, ok := readRequestChain(c)
					if !ok || !handle(c, cp) {
						return
					}
				}
			}(conn)
		}
	}()
	return srv
}

// readRequestChain reads one chained request and returns the codepoint of its first command.
func readRequestChain(c net.Conn) (drda.CodePoint, bool) {
	var first drda.CodePoint
	for i := 0; ; i++ {
		hdr, cp, _, _, err := drda.ReadDSS(c)
		if err != nil {
			return 0, false
		}
		if i == 0 {
			first = cp
		}
		if !hdr.Chained {
			return first, true
		}
	}
}

func mockHandshake(c net.Conn) bool {
	// EXCSAT + ACCSEC
	if _, ok := readRequestChain(c); !ok {
		return false
	}
	accsecRdObj := drda.PackDDMObject(drda.CodePointACCSECRD, drda.PackUint16(drda.CodePointSECMEC, drda.SecMecUSRIDPWD))
	if _, err := c.Write(append(drda.BuildDSSHeader(len(accsecRdObj), drda.DSSTypeReply, false, false, false, 1), accsecRdObj...)); err != nil {
		return false
	}

	// SECCHK + ACCRDB
	if _, ok := readRequestChain(c); !ok {
		return false
	}
	secchkRmObj := drda.PackDDMObject(drda.CodePointSECCHKRM, drda.PackBytes(drda.CodePointSECCHKCD, []byte{0x00}))
	accrdbRmObj := drda.PackDDMObject(drda.CodePointACCRDBRM, nil)
	var resp bytes.Buffer
	resp.Write(drda.BuildDSSHeader(len(secchkRmObj), drda.DSSTypeReply, true, false, false, 1))
	resp.Write(secchkRmObj)
	resp.Write(drda.BuildDSSHeader(len(accrdbRmObj), drda.DSSTypeReply, false, false, false, 1))
	resp.Write(accrdbRmObj)
	if _, err := c.Write(resp.Bytes()); err != nil {
		return false
	}

	// post-connect SET CLIENT / commit
	if _, ok := readRequestChain(c); !ok {
		return false
	}
	return writeSQLCARDOK(c)
}

// writeSQLCARDOK replies with a single SQLCARD carrying SQLCODE 0.
func writeSQLCARDOK(c net.Conn) bool {
	payload := make([]byte, 20)
	binary.LittleEndian.PutUint32(payload[1:5], 0)
	copy(payload[5:10], "00000")
	obj := drda.PackDDMObject(drda.CodePointSQLCARD, payload)
	_, err := c.Write(append(drda.BuildDSSHeader(len(obj), drda.DSSTypeReply, false, false, false, 1), obj...))
	return err == nil
}

// writeEXCSATRD replies with an EXCSATRD reply object.
func writeEXCSATRD(c net.Conn) bool {
	obj := drda.PackDDMObject(drda.CodePointEXCSATRD, nil)
	_, err := c.Write(append(drda.BuildDSSHeader(len(obj), drda.DSSTypeReply, false, false, false, 1), obj...))
	return err == nil
}

func writeEXCSATRDOrOK(c net.Conn) bool {
	return writeEXCSATRD(c)
}
