package network

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// queryScript is the server side of one query: the reply chain to OPNQRY,
// then one reply chain per CNTQRY, in order.
type queryScript struct {
	open      [][]byte
	continues [][][]byte
}

// serveQuery answers a single query on c according to script and reports how
// many CNTQRY requests the client sent. RDBCMM is answered with an empty SQLCARD.
func serveQuery(t *testing.T, c net.Conn, script queryScript, cntqrys chan<- int) {
	t.Helper()
	sent := 0
	defer func() { cntqrys <- sent }()
	for {
		var first CodePoint
		for i := 0; ; i++ {
			hdr, cp, _, _, err := ReadDSS(c)
			if err != nil {
				return
			}
			if i == 0 {
				first = cp
			}
			if !hdr.Chained {
				break
			}
		}
		var chain [][]byte
		switch first {
		case CodePointPRPSQLSTT, CodePointOPNQRY:
			chain = script.open
		case CodePointCNTQRY:
			if sent >= len(script.continues) {
				// The query is over: a client must not ask for more.
				sent++
				return
			}
			chain = script.continues[sent]
			sent++
		case CodePointRDBCMM:
			chain = [][]byte{PackDDMObject(CodePointSQLCARD, []byte{0xFF})}
		default:
			return
		}
		if _, err := c.Write(replyChain(chain...)); err != nil {
			return
		}
	}
}

// replyChain frames DDM objects as one chained reply.
func replyChain(objs ...[]byte) []byte {
	var out bytes.Buffer
	for i, obj := range objs {
		out.Write(replyDSS(obj, i < len(objs)-1, 1))
	}
	return out.Bytes()
}

func opnqryrm() []byte {
	body := PackUint16(CodePointSVRCOD, 0)
	body = append(body, PackBytes(CodePointQRYINSID, []byte{0, 0, 0, 0, 0, 0, 0, 1})...)
	return PackDDMObject(CodePointOPNQRYRM, body)
}

// qrydscInteger describes one non-nullable INTEGER column.
func qrydscInteger() []byte {
	return PackDDMObject(CodePointQRYDSC, []byte{0x06, 0x76, 0xD0, 0x02, 0x00, 0x04, 0x06, 0x71, 0xF0, 0xE0, 0x00, 0x00})
}

// integerRows encodes rows of one INTEGER column as QRYDTA row data.
func integerRows(values ...int32) []byte {
	var b []byte
	for _, v := range values {
		b = append(b, 0xFF, 0x00)
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	return b
}

func qrydta(data []byte) []byte { return PackDDMObject(CodePointQRYDTA, data) }

func sqlcard(code int32, state string) []byte {
	payload := make([]byte, 20)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(code))
	copy(payload[5:10], state)
	return PackDDMObject(CodePointSQLCARD, payload)
}

func endqryrm() []byte {
	return PackDDMObject(CodePointENDQRYRM, PackUint16(CodePointSVRCOD, 4))
}

func pipeSession(t *testing.T) (*Session, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	s := NewSession(SessionConfig{Host: "mock", Database: "TESTDB"})
	s.conn = client
	return s, server
}

func queryPaths() map[string]func(s *Session) ([][]driver.Value, error) {
	return map[string]func(s *Session) ([][]driver.Value, error){
		"QueryDirect": func(s *Session) ([][]driver.Value, error) {
			_, rows, err := s.QueryDirect(context.Background(), "SELECT N FROM T")
			return rows, err
		},
		"QueryWithParams": func(s *Session) ([][]driver.Value, error) {
			_, rows, err := s.QueryWithParams(context.Background(), nil, nil, nil)
			return rows, err
		},
	}
}

// A result set bigger than one query block arrives over several CNTQRY
// replies, and a row may start in one block and end in the next. The client
// has to keep continuing the query until the server ends it.
func TestQueryFetchesEveryQueryBlock(t *testing.T) {
	all := integerRows(1, 2, 3, 4, 5, 6, 7)
	split := len(integerRows(1, 2, 3)) + 3 // mid-way through row 4's value

	for name, query := range queryPaths() {
		t.Run(name, func(t *testing.T) {
			s, server := pipeSession(t)
			cntqrys := make(chan int, 1)
			go serveQuery(t, server, queryScript{
				open: [][]byte{opnqryrm(), qrydscInteger()},
				continues: [][][]byte{
					{qrydta(all[:split])},
					{qrydta(all[split : split+len(integerRows(0))])},
					{qrydta(all[split+len(integerRows(0)):]), endqryrm(), sqlcard(100, "02000")},
				},
			}, cntqrys)

			rows, err := query(s)
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			var got []int64
			for _, r := range rows {
				got = append(got, r[0].(int64))
			}
			if want := []int64{1, 2, 3, 4, 5, 6, 7}; !equalInt64(got, want) {
				t.Fatalf("rows = %v, want %v", got, want)
			}

			_ = s.conn.Close()
			select {
			case n := <-cntqrys:
				if n != 3 {
					t.Fatalf("client sent %d CNTQRY, want 3 (one per block, none after the query ended)", n)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mock server did not finish")
			}
		})
	}
}

// An error the server reports while the query is being continued must be
// returned, not dropped with the rows fetched so far.
func TestQueryReportsErrorsFromContinuedBlocks(t *testing.T) {
	for name, query := range queryPaths() {
		t.Run(name, func(t *testing.T) {
			s, server := pipeSession(t)
			cntqrys := make(chan int, 1)
			go serveQuery(t, server, queryScript{
				open: [][]byte{opnqryrm(), qrydscInteger()},
				continues: [][][]byte{
					{qrydta(integerRows(1, 2))},
					{sqlcard(-137, "54006"), endqryrm()},
				},
			}, cntqrys)

			_, err := query(s)
			if err == nil || !strings.Contains(err.Error(), "SQLCODE=-137") {
				t.Fatalf("expected the SQLCODE -137 from the second block, got: %v", err)
			}
		})
	}
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
