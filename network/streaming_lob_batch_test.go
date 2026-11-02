package network

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-db2/go-db2/types"
)

// TestQueryStream_RowSpanningAndDemandPaging tests that QueryStream decodes partial rows,
// preserves the tail across QRYDTA blocks, and only issues CNTQRY when requested.
func TestQueryStream_RowSpanningAndDemandPaging(t *testing.T) {
	all := integerRows(10, 20, 30, 40)
	// Split in the middle of row 3 (each row is 6 bytes: 2 bytes null ind + 4 bytes int32)
	// Row 1: 0..6, Row 2: 6..12, Row 3: 12..18 (split at 15)
	split := 15

	s, server := pipeSession(t)
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)
		for {
			hdr, cp, _, _, err := ReadDSS(server)
			if err != nil {
				return
			}
			for hdr.Chained {
				h, _, _, _, err := ReadDSS(server)
				if err != nil {
					return
				}
				hdr = h
			}

			switch cp {
			case CodePointPRPSQLSTT, CodePointOPNQRY:
				// Reply with OPNQRYRM, QRYDSC, and QRYDTA (first chunk: rows 1, 2, and half of 3)
				reply1 := replyChain(opnqryrm(), qrydscInteger(), qrydta(all[:split]))
				if _, err := server.Write(reply1); err != nil {
					return
				}
			case CodePointCNTQRY:
				// Reply with second chunk: rest of row 3 and row 4, plus ENDQRYRM
				reply2 := replyChain(qrydta(all[split:]), endqryrm(), sqlcard(100, "02000"))
				if _, err := server.Write(reply2); err != nil {
					return
				}
			case CodePointRDBCMM:
				_, _ = server.Write(replyChain(sqlcard(0, "00000")))
				return
			}
		}
	}()

	cols, initialRows, qs, err := s.QueryStreamDirect(context.Background(), "SELECT ID FROM T")
	if err != nil {
		t.Fatalf("QueryStreamDirect failed: %v", err)
	}
	defer func() { _ = qs.Close(context.Background()) }()

	if len(cols) != 1 || cols[0].Name != "COL1" {
		t.Logf("Cols: %+v", cols)
	}

	// Initial batch must have decoded complete rows 1 and 2 only (row 3 was split)
	if len(initialRows) != 2 {
		t.Fatalf("expected initial batch to contain 2 complete rows, got %d", len(initialRows))
	}
	if initialRows[0][0].(int64) != 10 || initialRows[1][0].(int64) != 20 {
		t.Fatalf("initial rows mismatch: got %v", initialRows)
	}

	// Fetch next batch via CNTQRY
	nextBatch, err := qs.FetchNextBatch(context.Background())
	if err != nil {
		t.Fatalf("FetchNextBatch failed: %v", err)
	}

	// Must contain rows 3 and 4 stitched correctly
	if len(nextBatch) != 2 {
		t.Fatalf("expected next batch to contain 2 rows, got %d", len(nextBatch))
	}
	if nextBatch[0][0].(int64) != 30 || nextBatch[1][0].(int64) != 40 {
		t.Fatalf("stitched rows mismatch: got %v", nextBatch)
	}

	// Next fetch must return EOF
	_, err = qs.FetchNextBatch(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF after end of query, got %v", err)
	}

	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("mock server timeout")
	}
}

// TestQueryStream_EarlyClose_EmitsCLSQRY tests that closing an open query stream before ENDQRYRM
// emits a CLSQRY command on the socket.
func TestQueryStream_EarlyClose_EmitsCLSQRY(t *testing.T) {
	all := integerRows(1, 2, 3, 4, 5)

	s, server := pipeSession(t)
	clsqrySeen := make(chan bool, 1)

	go func() {
		defer close(clsqrySeen)
		for {
			hdr, cp, _, _, err := ReadDSS(server)
			if err != nil {
				return
			}
			for hdr.Chained {
				h, _, _, _, err := ReadDSS(server)
				if err != nil {
					return
				}
				hdr = h
			}

			switch cp {
			case CodePointPRPSQLSTT, CodePointOPNQRY:
				reply1 := replyChain(opnqryrm(), qrydscInteger(), qrydta(all[:12]))
				if _, err := server.Write(reply1); err != nil {
					return
				}
			case CodePointCLSQRY:
				clsqrySeen <- true
				_, _ = server.Write(replyChain(sqlcard(0, "00000")))
			case CodePointRDBCMM:
				_, _ = server.Write(replyChain(sqlcard(0, "00000")))
				return
			}
		}
	}()

	_, initialRows, qs, err := s.QueryStreamDirect(context.Background(), "SELECT ID FROM T")
	if err != nil {
		t.Fatalf("QueryStreamDirect failed: %v", err)
	}
	if len(initialRows) != 2 {
		t.Fatalf("expected 2 initial rows, got %d", len(initialRows))
	}

	// Close stream early
	if err := qs.Close(context.Background()); err != nil {
		t.Fatalf("qs.Close failed: %v", err)
	}

	select {
	case seen := <-clsqrySeen:
		if !seen {
			t.Fatal("expected CLSQRY codepoint from client on early close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for CLSQRY on mock server")
	}
}

// TestSession_ExecBatchWithParams_Atomicity tests batch execution atomicity:
// on failure, issues Rollback (RDBRLLBCK) and does not commit previous rows.
func TestSession_ExecBatchWithParams_Atomicity(t *testing.T) {
	paramCols := []ColumnDescription{
		{Name: "VAL", SQLType: uint16(types.SQLTypeInteger)},
	}

	t.Run("RollbackOnRowFailure", func(t *testing.T) {
		s, server := pipeSession(t)
		s.SetAutoCommit(true)

		rollbackSeen := make(chan bool, 1)

		go func() {
			defer close(rollbackSeen)
			rowCount := 0
			for {
				hdr, cp, _, _, err := ReadDSS(server)
				if err != nil {
					return
				}
				for hdr.Chained {
					h, _, _, _, err := ReadDSS(server)
					if err != nil {
						return
					}
					hdr = h
				}

				if cp == CodePointEXCSQLSTT {
					rowCount++
					if rowCount == 1 {
						// Row 1 success
						_, _ = server.Write(replyChain(sqlcard(0, "00000")))
					} else {
						// Row 2 error (e.g., -803 unique constraint violation)
						_, _ = server.Write(replyChain(sqlcard(-803, "23505")))
					}
				} else if cp == CodePointRDBRLLBCK {
					rollbackSeen <- true
					_, _ = server.Write(replyChain(sqlcard(0, "00000")))
					return
				} else if cp == CodePointRDBCMM {
					t.Errorf("unexpected RDBCMM received during failing batch!")
					return
				}
			}
		}()

		batch := [][]any{{1}, {2}}
		affected, err := s.ExecBatchWithParams(context.Background(), ReservedDirectSec, paramCols, batch)
		if err == nil || !strings.Contains(err.Error(), "rolled back") {
			t.Fatalf("expected rolled back error, got affected=%d, err=%v", affected, err)
		}

		select {
		case seen := <-rollbackSeen:
			if !seen {
				t.Fatal("expected RDBRLLBCK on mock server")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for RDBRLLBCK")
		}

		if !s.AutoCommit() {
			t.Errorf("expected autoCommit to be restored to true after batch")
		}
	})

	t.Run("SingleCommitOnSuccess", func(t *testing.T) {
		s, server := pipeSession(t)
		s.SetAutoCommit(true)

		commitSeen := make(chan bool, 1)

		go func() {
			defer close(commitSeen)
			for {
				hdr, cp, _, _, err := ReadDSS(server)
				if err != nil {
					return
				}
				for hdr.Chained {
					h, _, _, _, err := ReadDSS(server)
					if err != nil {
						return
					}
					hdr = h
				}

				if cp == CodePointEXCSQLSTT {
					// Return success for each row
					_, _ = server.Write(replyChain(sqlcard(0, "00000")))
				} else if cp == CodePointRDBCMM {
					commitSeen <- true
					_, _ = server.Write(replyChain(sqlcard(0, "00000")))
					return
				}
			}
		}()

		batch := [][]any{{10}, {20}, {30}}
		affected, err := s.ExecBatchWithParams(context.Background(), ReservedDirectSec, paramCols, batch)
		if err != nil {
			t.Fatalf("unexpected error on successful batch: %v", err)
		}
		if affected != 0 {
			t.Logf("affected: %d", affected)
		}

		select {
		case seen := <-commitSeen:
			if !seen {
				t.Fatal("expected RDBCMM on mock server after successful batch")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for RDBCMM")
		}

		if !s.AutoCommit() {
			t.Errorf("expected autoCommit to be true after batch")
		}
	})
}

// TestSession_QueryCallDirect_MultiResultSets tests decoding multiple result sets from a CALL statement.
func TestSession_QueryCallDirect_MultiResultSets(t *testing.T) {
	s, server := pipeSession(t)
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)
		// Read EXCSQLIMM + SQLSTT
		for {
			hdr, cp, _, _, err := ReadDSS(server)
			if err != nil {
				return
			}
			if cp == CodePointSQLSTT || !hdr.Chained {
				break
			}
		}

		// Two result sets chained in reply:
		// Result Set 1: Integer column, rows [10, 20]
		// Result Set 2: Integer column, rows [30, 40]
		reply := replyChain(
			opnqryrm(),
			qrydscInteger(),
			qrydta(integerRows(10, 20)),
			qrydscInteger(),
			qrydta(integerRows(30, 40)),
			sqlcard(0, "00000"),
		)
		_, _ = server.Write(reply)
	}()

	sets, err := s.QueryCallDirect(context.Background(), "CALL MULTI_PROC()")
	if err != nil {
		t.Fatalf("QueryCallDirect failed: %v", err)
	}

	if len(sets) != 2 {
		t.Fatalf("expected 2 result sets, got %d", len(sets))
	}

	if len(sets[0].Data) != 2 || sets[0].Data[0][0].(int64) != 10 || sets[0].Data[1][0].(int64) != 20 {
		t.Errorf("set 0 mismatch: got %v", sets[0].Data)
	}
	if len(sets[1].Data) != 2 || sets[1].Data[0][0].(int64) != 30 || sets[1].Data[1][0].(int64) != 40 {
		t.Errorf("set 1 mismatch: got %v", sets[1].Data)
	}

	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Fatal("mock server timeout")
	}
}
