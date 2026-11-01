package db2

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync/atomic"
	"testing"

	"github.com/go-db2/go-db2/network"
)

// mockServer is a minimal DRDA server: it completes the connect handshake and
// then hands every request chain to handle, identified by the codepoint of
// the chain's first command. handle returns false to drop the connection.
type mockServer struct {
	addr    string
	accepts atomic.Int32
}

func startMockServer(t *testing.T, handle func(c net.Conn, cp network.CodePoint) bool) *mockServer {
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
func readRequestChain(c net.Conn) (network.CodePoint, bool) {
	var first network.CodePoint
	for i := 0; ; i++ {
		hdr, cp, _, _, err := network.ReadDSS(c)
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
	accsecRdObj := network.PackDDMObject(network.CodePointACCSECRD, network.PackUint16(network.CodePointSECMEC, network.SecMecUSRIDPWD))
	if _, err := c.Write(append(network.BuildDSSHeader(len(accsecRdObj), network.DSSTypeReply, false, false, false, 1), accsecRdObj...)); err != nil {
		return false
	}

	// SECCHK + ACCRDB
	if _, ok := readRequestChain(c); !ok {
		return false
	}
	secchkRmObj := network.PackDDMObject(network.CodePointSECCHKRM, network.PackBytes(network.CodePointSECCHKCD, []byte{0x00}))
	accrdbRmObj := network.PackDDMObject(network.CodePointACCRDBRM, nil)
	var resp bytes.Buffer
	resp.Write(network.BuildDSSHeader(len(secchkRmObj), network.DSSTypeReply, true, false, false, 1))
	resp.Write(secchkRmObj)
	resp.Write(network.BuildDSSHeader(len(accrdbRmObj), network.DSSTypeReply, false, false, false, 1))
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
	obj := network.PackDDMObject(network.CodePointSQLCARD, payload)
	_, err := c.Write(append(network.BuildDSSHeader(len(obj), network.DSSTypeReply, false, false, false, 1), obj...))
	return err == nil
}

// writeEXCSATRD replies with an EXCSATRD reply object.
func writeEXCSATRD(c net.Conn) bool {
	obj := network.PackDDMObject(network.CodePointEXCSATRD, nil)
	_, err := c.Write(append(network.BuildDSSHeader(len(obj), network.DSSTypeReply, false, false, false, 1), obj...))
	return err == nil
}

func writeEXCSATRDOrOK(c net.Conn) bool {
	return writeEXCSATRD(c)
}
