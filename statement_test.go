package db2

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-db2/go-db2/internal/codec"
	"github.com/go-db2/go-db2/internal/drda"
)

func TestHasBlobParams(t *testing.T) {
	tests := []struct {
		name      string
		paramCols []drda.ColumnDescription
		args      []any
		want      bool
	}{
		{
			name:      "Empty paramCols and args",
			paramCols: nil,
			args:      nil,
			want:      false,
		},
		{
			name: "Non-BLOB column with []byte argument",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeVarChar)},
			},
			args: []any{
				[]byte("hello"),
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-empty []byte",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				[]byte{0x01, 0x02, 0x03},
			},
			want: true,
		},
		{
			name: "SQLTypeNBlob with non-empty []byte",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeNBlob)},
			},
			args: []any{
				[]byte{0xDE, 0xAD, 0xBE, 0xEF},
			},
			want: true,
		},
		{
			name: "SQLTypeBlob with empty []byte",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				[]byte{},
			},
			want: false,
		},
		{
			name: "SQLTypeNBlob with empty []byte",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeNBlob)},
			},
			args: []any{
				[]byte{},
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-[]byte argument (string)",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				"not bytes",
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-[]byte argument (int)",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				12345,
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with nil argument",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				nil,
			},
			want: false,
		},
		{
			name: "Bounds check: paramCols longer than args where BLOB is out of bounds",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger)},
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				10,
			},
			want: false,
		},
		{
			name: "Multiple parameters where BLOB parameter is at index > 0",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger)},
				{SQLType: uint16(codec.SQLTypeVarChar)},
				{SQLType: uint16(codec.SQLTypeBlob)},
			},
			args: []any{
				1,
				"test",
				[]byte{0xFF, 0xFE},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBlobParams(tt.paramCols, tt.args)
			if got != tt.want {
				t.Errorf("hasBlobParams() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssignOutParam_NilSafety(t *testing.T) {
	// 1. Untyped nil dest
	if err := assignOutParam(nil, "value"); err != nil {
		t.Errorf("assignOutParam(nil, 'value') = %v, want nil", err)
	}

	// 2. Untyped nil val
	var target int
	if err := assignOutParam(&target, nil); err != nil {
		t.Errorf("assignOutParam(&target, nil) = %v, want nil", err)
	}

	// 3. Non-pointer dest
	if err := assignOutParam(target, 42); err == nil {
		t.Errorf("assignOutParam(non-pointer) expected error, got nil")
	}

	// 4. Typed nil pointer dest
	var nilPtr *int
	if err := assignOutParam(nilPtr, 42); err == nil {
		t.Errorf("assignOutParam(typed nil ptr) expected error, got nil")
	}
}

func TestRewriteBinaryParams(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		paramCols         []drda.ColumnDescription
		args              []any
		wantQuery         string
		wantRewrittenCols []drda.ColumnDescription
		wantRewrittenArgs []any
	}{
		{
			name:              "No parameters",
			query:             "SELECT * FROM users WHERE id = 1",
			paramCols:         []drda.ColumnDescription{},
			args:              []any{},
			wantQuery:         "SELECT * FROM users WHERE id = 1",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Non-BLOB parameters only",
			query: "INSERT INTO users (id, name) VALUES (?, ?)",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(codec.SQLTypeVarChar), Name: "NAME"},
			},
			args:      []any{1, "Alice"},
			wantQuery: "INSERT INTO users (id, name) VALUES (?, ?)",
			wantRewrittenCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(codec.SQLTypeVarChar), Name: "NAME"},
			},
			wantRewrittenArgs: []any{1, "Alice"},
		},
		{
			name:  "BLOB parameter rewritten to BLOB(X'...')",
			query: "INSERT INTO files (id, data) VALUES (?, ?)",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:      []any{42, []byte{0xAB, 0xCD, 0xEF}},
			wantQuery: "INSERT INTO files (id, data) VALUES (?, BLOB(X'abcdef'))",
			wantRewrittenCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
			},
			wantRewrittenArgs: []any{42},
		},
		{
			name:  "SQLTypeNBlob parameter rewritten to BLOB(X'...')",
			query: "UPDATE files SET data = ? WHERE id = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeNBlob), Name: "DATA"},
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
			},
			args:      []any{[]byte{0x01, 0x02}, 10},
			wantQuery: "UPDATE files SET data = BLOB(X'0102') WHERE id = ?",
			wantRewrittenCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
			},
			wantRewrittenArgs: []any{10},
		},
		{
			name:  "BLOB with empty []byte is kept as parameter",
			query: "INSERT INTO files (data) VALUES (?)",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:      []any{[]byte{}},
			wantQuery: "INSERT INTO files (data) VALUES (?)",
			wantRewrittenCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			wantRewrittenArgs: []any{[]byte{}},
		},
		{
			name:  "Ignore ? in single-quoted string literal",
			query: "SELECT * FROM t WHERE note = 'Is this ? real?' AND data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x12}},
			wantQuery:         "SELECT * FROM t WHERE note = 'Is this ? real?' AND data = BLOB(X'12')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in escaped single-quoted string literal",
			query: "SELECT * FROM t WHERE note = 'It''s a ? test' AND data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x12}},
			wantQuery:         "SELECT * FROM t WHERE note = 'It''s a ? test' AND data = BLOB(X'12')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in double-quoted identifier",
			query: "SELECT * FROM \"col?name\" WHERE data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x34}},
			wantQuery:         "SELECT * FROM \"col?name\" WHERE data = BLOB(X'34')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in escaped double-quoted identifier",
			query: "SELECT * FROM \"col\"\"?name\" WHERE data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x34}},
			wantQuery:         "SELECT * FROM \"col\"\"?name\" WHERE data = BLOB(X'34')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in single-line comment",
			query: "SELECT 1 -- Is this a question mark ? \n FROM t WHERE data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x56}},
			wantQuery:         "SELECT 1 -- Is this a question mark ? \n FROM t WHERE data = BLOB(X'56')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in block comment",
			query: "SELECT /* ? comment ? */ * FROM t WHERE data = ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x78}},
			wantQuery:         "SELECT /* ? comment ? */ * FROM t WHERE data = BLOB(X'78')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Extra ? in query beyond paramCols length",
			query: "SELECT ?, ?",
			paramCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
			},
			args:      []any{1},
			wantQuery: "SELECT ?, ?",
			wantRewrittenCols: []drda.ColumnDescription{
				{SQLType: uint16(codec.SQLTypeInteger), Name: "ID"},
			},
			wantRewrittenArgs: []any{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotCols, gotArgs := rewriteBinaryParams(tt.query, tt.paramCols, tt.args)
			if gotQuery != tt.wantQuery {
				t.Errorf("rewriteBinaryParams() gotQuery = %q, want %q", gotQuery, tt.wantQuery)
			}
			if !reflect.DeepEqual(gotCols, tt.wantRewrittenCols) {
				t.Errorf("rewriteBinaryParams() gotCols = %#v, want %#v", gotCols, tt.wantRewrittenCols)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantRewrittenArgs) {
				t.Errorf("rewriteBinaryParams() gotArgs = %#v, want %#v", gotArgs, tt.wantRewrittenArgs)
			}
		})
	}
}

func BenchmarkSectionManager_AcquireRelease(b *testing.B) {
	sm := newSectionManager(500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sec, err := sm.Acquire()
		if err != nil {
			b.Fatal(err)
		}
		sm.Release(sec)
	}
}

type customTestUUID [16]byte

func TestFixedByteArrayNormalizationAndBatch(t *testing.T) {
	t.Run("CheckNamedValueCommon", func(t *testing.T) {
		arr16 := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		arr32 := [32]byte{0xAA, 0xBB, 0xCC}
		arr8 := [8]byte{9, 8, 7, 6, 5, 4, 3, 2}
		named := customTestUUID{0x10, 0x20, 0x30}

		nv16 := &driver.NamedValue{Ordinal: 1, Value: arr16}
		if err := checkNamedValueCommon(nv16); err != nil {
			t.Fatalf("[16]byte checkNamedValueCommon error: %v", err)
		}
		got16, ok := nv16.Value.([]byte)
		if !ok || !bytes.Equal(got16, arr16[:]) {
			t.Fatalf("expected []byte matching arr16, got %#v (%T)", nv16.Value, nv16.Value)
		}

		ptr16 := arr16
		nvPtr16 := &driver.NamedValue{Ordinal: 1, Value: &ptr16}
		if err := checkNamedValueCommon(nvPtr16); err != nil {
			t.Fatalf("*[16]byte checkNamedValueCommon error: %v", err)
		}
		ptr16[0] = 0xFF
		gotPtr16, ok := nvPtr16.Value.([]byte)
		if !ok || gotPtr16[0] != 1 {
			t.Fatalf("expected defensive copy for *[16]byte, got %#v", nvPtr16.Value)
		}

		var nilPtr16 *[16]byte
		nvNil16 := &driver.NamedValue{Ordinal: 1, Value: nilPtr16}
		if err := checkNamedValueCommon(nvNil16); err != nil {
			t.Fatalf("(*[16]byte)(nil) checkNamedValueCommon error: %v", err)
		}
		if nvNil16.Value != nil {
			t.Fatalf("expected untyped nil for (*[16]byte)(nil), got %#v (%T)", nvNil16.Value, nvNil16.Value)
		}

		nv32 := &driver.NamedValue{Ordinal: 1, Value: arr32}
		if err := checkNamedValueCommon(nv32); err != nil {
			t.Fatalf("[32]byte checkNamedValueCommon error: %v", err)
		}
		got32, ok := nv32.Value.([]byte)
		if !ok || len(got32) != 32 || !bytes.Equal(got32, arr32[:]) {
			t.Fatalf("expected []byte(32), got %#v (%T)", nv32.Value, nv32.Value)
		}

		nvPtr8 := &driver.NamedValue{Ordinal: 1, Value: &arr8}
		if err := checkNamedValueCommon(nvPtr8); err != nil {
			t.Fatalf("*[8]byte checkNamedValueCommon error: %v", err)
		}
		got8, ok := nvPtr8.Value.([]byte)
		if !ok || !bytes.Equal(got8, arr8[:]) {
			t.Fatalf("expected []byte(8), got %#v (%T)", nvPtr8.Value, nvPtr8.Value)
		}

		var nilPtr8 *[8]byte
		nvNil8 := &driver.NamedValue{Ordinal: 1, Value: nilPtr8}
		if err := checkNamedValueCommon(nvNil8); err != nil {
			t.Fatalf("(*[8]byte)(nil) checkNamedValueCommon error: %v", err)
		}
		if nvNil8.Value != nil {
			t.Fatalf("expected untyped nil for (*[8]byte)(nil), got %#v (%T)", nvNil8.Value, nvNil8.Value)
		}

		nvNamed := &driver.NamedValue{Ordinal: 1, Value: named}
		if err := checkNamedValueCommon(nvNamed); err != nil {
			t.Fatalf("customTestUUID checkNamedValueCommon error: %v", err)
		}
		if got, ok := nvNamed.Value.([]byte); !ok || !bytes.Equal(got, named[:]) {
			t.Fatalf("expected []byte for customTestUUID, got %#v (%T)", nvNamed.Value, nvNamed.Value)
		}

		nvNamedPtr := &driver.NamedValue{Ordinal: 1, Value: &named}
		if err := checkNamedValueCommon(nvNamedPtr); err != nil {
			t.Fatalf("*customTestUUID checkNamedValueCommon error: %v", err)
		}
		if got, ok := nvNamedPtr.Value.([]byte); !ok || !bytes.Equal(got, named[:]) {
			t.Fatalf("expected []byte for *customTestUUID, got %#v (%T)", nvNamedPtr.Value, nvNamedPtr.Value)
		}

		var nilNamed *customTestUUID
		nvNilNamed := &driver.NamedValue{Ordinal: 1, Value: nilNamed}
		if err := checkNamedValueCommon(nvNilNamed); err != nil {
			t.Fatalf("(*customTestUUID)(nil) checkNamedValueCommon error: %v", err)
		}
		if nvNilNamed.Value != nil {
			t.Fatalf("expected untyped nil for (*customTestUUID)(nil), got %#v", nvNilNamed.Value)
		}

		nvBatchBytes := &driver.NamedValue{Ordinal: 1, Value: [][]byte{[]byte("a"), []byte("b")}}
		if err := checkNamedValueCommon(nvBatchBytes); err != nil {
			t.Fatalf("expected [][]byte to return nil, got %v", err)
		}

		nvBatchUUID := &driver.NamedValue{Ordinal: 1, Value: [][16]byte{{1}, {2}}}
		if err := checkNamedValueCommon(nvBatchUUID); err != nil {
			t.Fatalf("expected [][16]byte to return nil, got %v", err)
		}
	})

	t.Run("GetSliceLenAndDetectAndExtractBatch", func(t *testing.T) {
		arr16 := [16]byte{1, 2, 3}
		arr32 := [32]byte{4, 5, 6}
		arr8 := [8]byte{7, 8}
		named := customTestUUID{9, 10}
		var nilPtr16 *[16]byte
		var nilPtr8 *[8]byte

		for _, scalar := range []any{arr16, &arr16, nilPtr16, arr32, arr8, &arr8, nilPtr8, named, &named, []byte("abc")} {
			if n, isSlice := getSliceLen(scalar); isSlice || n != 0 {
				t.Fatalf("expected scalar %T not to be treated as batch slice, got n=%d isSlice=%v", scalar, n, isSlice)
			}
		}

		if n, isSlice := getSliceLen([][]byte{[]byte("a"), []byte("b")}); !isSlice || n != 2 {
			t.Fatalf("expected [][]byte slice len=2, got n=%d isSlice=%v", n, isSlice)
		}

		// Batch size 40 (> 32 and > 16) with scalar [16]byte, *[16]byte, [32]byte, customTestUUID, typed nil, [][]byte, [][16]byte
		ids := make([]int, 40)
		blobs := make([][]byte, 40)
		uuids := make([]customTestUUID, 40)
		for i := range ids {
			ids[i] = i + 1
			if i%2 == 0 {
				blobs[i] = []byte{byte(i)}
			}
			uuids[i] = customTestUUID{byte(i)}
		}

		isBatch, rows, err := detectAndExtractBatch([]any{ids, arr16, &arr16, arr32, named, nilPtr8, blobs, uuids})
		if err != nil {
			t.Fatalf("detectAndExtractBatch failed: %v", err)
		}
		if !isBatch || len(rows) != 40 {
			t.Fatalf("expected batch of 40 rows, got isBatch=%v len=%d", isBatch, len(rows))
		}
		for r := range rows {
			if b, ok := rows[r][1].([]byte); !ok || !bytes.Equal(b, arr16[:]) {
				t.Fatalf("row %d col 1 expected []byte(arr16), got %#v (%T)", r, rows[r][1], rows[r][1])
			}
			if b, ok := rows[r][2].([]byte); !ok || !bytes.Equal(b, arr16[:]) {
				t.Fatalf("row %d col 2 expected []byte(*arr16), got %#v (%T)", r, rows[r][2], rows[r][2])
			}
			if b, ok := rows[r][3].([]byte); !ok || len(b) != 32 || !bytes.Equal(b, arr32[:]) {
				t.Fatalf("row %d col 3 expected []byte(arr32), got %#v (%T)", r, rows[r][3], rows[r][3])
			}
			if b, ok := rows[r][4].([]byte); !ok || !bytes.Equal(b, named[:]) {
				t.Fatalf("row %d col 4 expected []byte(customTestUUID), got %#v (%T)", r, rows[r][4], rows[r][4])
			}
			if rows[r][5] != nil {
				t.Fatalf("row %d col 5 expected untyped nil for (*[8]byte)(nil), got %#v (%T)", r, rows[r][5], rows[r][5])
			}
			if r%2 == 0 {
				if b, ok := rows[r][6].([]byte); !ok || len(b) != 1 || b[0] != byte(r) {
					t.Fatalf("row %d col 6 mismatch: %#v", r, rows[r][6])
				}
			} else if b, ok := rows[r][6].([]byte); !ok || b != nil {
				t.Fatalf("row %d col 6 expected []byte(nil) element in [][]byte, got %#v (%T)", r, rows[r][6], rows[r][6])
			}
			if b, ok := rows[r][7].([]byte); !ok || len(b) != 16 || b[0] != byte(r) {
				t.Fatalf("row %d col 7 expected normalized []byte from []customTestUUID, got %#v (%T)", r, rows[r][7], rows[r][7])
			}
		}
	})

	t.Run("StmtExecAndExecContext_BatchWithFixedByteArrays", func(t *testing.T) {
		var mu sync.Mutex
		var excsqlsttCount int

		srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
			switch cp {
			case drda.CodePointEXCSQLSTT:
				mu.Lock()
				excsqlsttCount++
				mu.Unlock()
				return writeSQLCARDOK(c)
			case drda.CodePointRDBCMM, drda.CodePointEXCSQLSET:
				return writeSQLCARDOK(c)
			}
			return false
		})

		db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sqlConn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("db.Conn failed: %v", err)
		}
		defer sqlConn.Close()

		var conn *Conn
		if err := sqlConn.Raw(func(raw any) error {
			conn = raw.(*Conn)
			return nil
		}); err != nil || conn == nil {
			t.Fatalf("failed to extract *Conn: %v", err)
		}

		stmt := newStmt(conn, "INSERT INTO T VALUES (?, ?, ?, ?, ?)", nil, []drda.ColumnDescription{
			{SQLType: uint16(codec.SQLTypeInteger), Length: 4},
			{SQLType: uint16(codec.SQLTypeBlob), Length: 16},
			{SQLType: uint16(codec.SQLTypeBlob), Length: 16},
			{SQLType: uint16(codec.SQLTypeBlob), Length: 32},
			{SQLType: uint16(codec.SQLTypeBlob), Length: 64},
		}, 1)

		ids := make([]int, 40)
		payloads := make([][]byte, 40)
		for i := range ids {
			ids[i] = i + 1
			payloads[i] = []byte("row-payload")
		}
		arr16 := [16]byte{1, 2, 3, 4}
		named := customTestUUID{5, 6, 7, 8}
		arr32 := [32]byte{9, 10, 11, 12}

		// Stmt.ExecContext path (after CheckNamedValue)
		namedArgs := []driver.NamedValue{
			{Ordinal: 1, Value: ids},
			{Ordinal: 2, Value: arr16},
			{Ordinal: 3, Value: named},
			{Ordinal: 4, Value: arr32},
			{Ordinal: 5, Value: payloads},
		}
		for i := range namedArgs {
			if err := stmt.CheckNamedValue(&namedArgs[i]); err != nil {
				t.Fatalf("CheckNamedValue arg %d failed: %v", i, err)
			}
		}
		res, err := stmt.ExecContext(ctx, namedArgs)
		if err != nil {
			t.Fatalf("Stmt.ExecContext batch failed: %v", err)
		}
		if aff, _ := res.RowsAffected(); aff != 40 {
			t.Fatalf("expected 40 rows affected from ExecContext, got %d", aff)
		}

		// Stmt.Exec legacy driver.Value path
		legacyArgs := []driver.Value{ids, arr16, named, arr32, payloads}
		res2, err := stmt.Exec(legacyArgs)
		if err != nil {
			t.Fatalf("Stmt.Exec batch failed: %v", err)
		}
		if aff, _ := res2.RowsAffected(); aff != 40 {
			t.Fatalf("expected 40 rows affected from Stmt.Exec, got %d", aff)
		}

		mu.Lock()
		gotExecs := excsqlsttCount
		mu.Unlock()
		if gotExecs != 80 {
			t.Fatalf("expected 80 EXCSQLSTT dispatches across two 40-row batches, got %d", gotExecs)
		}
	})
}

func TestStmt_RewriteBinaryParamsAndOutMapping(t *testing.T) {
	t.Run("Unit_HasBlobInParamsAndRewriteBinaryParamsMapped", func(t *testing.T) {
		cols := []drda.ColumnDescription{
			{SQLType: uint16(codec.SQLTypeBlob)},
			{SQLType: uint16(codec.SQLTypeBlob)},
			{SQLType: uint16(codec.SQLTypeInteger)},
			{SQLType: uint16(codec.SQLTypeBlob)},
		}

		smallIn := []byte{0xca, 0xfe}
		outBlob := []byte{0x01, 0x02}
		oversizeBlob := bytes.Repeat([]byte{0x99}, 16384)
		args := []any{smallIn, outBlob, int32(7), oversizeBlob}
		outIndices := map[int]bool{1: true, 2: true}

		if !hasBlobInParams(cols, args, outIndices) {
			t.Fatal("expected hasBlobInParams=true for non-OUT small BLOB at idx 0")
		}
		if hasBlobInParams([]drda.ColumnDescription{{SQLType: uint16(codec.SQLTypeBlob)}}, []any{outBlob}, map[int]bool{0: true}) {
			t.Fatal("expected hasBlobInParams=false when the only small BLOB is sql.Out")
		}

		q, keptCols, keptArgs, idxMap := rewriteBinaryParamsMapped("CALL P(?, ?, ?, ?)", cols, args, outIndices)
		wantQ := "CALL P(BLOB(X'cafe'), ?, ?, ?)"
		if q != wantQ {
			t.Fatalf("rewritten query = %q, want %q", q, wantQ)
		}
		if len(keptCols) != 3 || len(keptArgs) != 3 {
			t.Fatalf("expected 3 remaining params (sql.Out BLOB, sql.Out INT, oversize BLOB), got %d cols / %d args", len(keptCols), len(keptArgs))
		}
		if idxMap[1] != 0 || idxMap[2] != 1 || idxMap[3] != 2 {
			t.Fatalf("unexpected indexMap: %#v", idxMap)
		}
		if _, inlinedExists := idxMap[0]; inlinedExists {
			t.Fatalf("inlined param 0 should not be in indexMap: %#v", idxMap)
		}
	})

	t.Run("MockServer_StmtLifecycleAndOutRemapping", func(t *testing.T) {
		var mu sync.Mutex
		var ops []drda.CodePoint
		returnOutSQLDTARD := false
		failPrepare := false

		// Build palindromic int32 (0x11222211 = 287449617) SQLDTARD payload (endian-neutral value)
		fdodscPayload := []byte{
			0x06, 0x76, 0xD0,
			0x02, 0x00, 0x04,
			0x06, 0x71, 0xE4, 0xD0, 0x00, 0x01,
		}
		var fdodscObj []byte
		fdodscObj = binary.BigEndian.AppendUint16(fdodscObj, uint16(4+len(fdodscPayload)))
		fdodscObj = binary.BigEndian.AppendUint16(fdodscObj, 0x0010)
		fdodscObj = append(fdodscObj, fdodscPayload...)

		fdodtaPayload := []byte{0xFF, 0x00, 0x11, 0x22, 0x22, 0x11}
		var fdodtaObj []byte
		fdodtaObj = binary.BigEndian.AppendUint16(fdodtaObj, uint16(4+len(fdodtaPayload)))
		fdodtaObj = binary.BigEndian.AppendUint16(fdodtaObj, 0x147A)
		fdodtaObj = append(fdodtaObj, fdodtaPayload...)
		sqldtardPayload := append(fdodscObj, fdodtaObj...)

		srv := startMockServer(t, func(c net.Conn, cp drda.CodePoint) bool {
			mu.Lock()
			ops = append(ops, cp)
			sendOut := returnOutSQLDTARD
			dropPrep := failPrepare
			mu.Unlock()

			switch cp {
			case drda.CodePointPRPSQLSTT:
				if dropPrep {
					return false
				}
				return writeSQLCARDOK(c)
			case drda.CodePointEXCSQLIMM, drda.CodePointRDBCMM, drda.CodePointRDBRLLBCK:
				return writeSQLCARDOK(c)
			case drda.CodePointEXCSQLSTT:
				if sendOut {
					_, _ = c.Write(buildReplyChain(
						drda.PackDDMObject(drda.CodePointSQLDTARD, sqldtardPayload),
						mockSqlcard(1, "00000"),
					))
					return true
				}
				return writeSQLCARDOK(c)
			}
			return false
		})

		db, err := sql.Open("db2", "db2://user:pass@"+srv.addr+"/TESTDB?ssl=false&allow_plaintext_auth=true")
		if err != nil {
			t.Fatalf("sql.Open failed: %v", err)
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sqlConn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("db.Conn failed: %v", err)
		}
		defer sqlConn.Close()

		var conn *Conn
		if err := sqlConn.Raw(func(raw any) error {
			conn = raw.(*Conn)
			return nil
		}); err != nil || conn == nil {
			t.Fatalf("failed to extract *Conn: %v", err)
		}

		// 1. All-BLOB rewrite uses ExecDirect (EXCSQLIMM) and does NOT mark rewrittenOnServer=true
		stmtDirect := newStmt(conn, "INSERT INTO T VALUES (?)", nil, []drda.ColumnDescription{
			{SQLType: uint16(codec.SQLTypeBlob), Length: 1024},
		}, 1)

		mu.Lock()
		ops = nil
		mu.Unlock()

		if _, err := stmtDirect.ExecContext(ctx, []driver.NamedValue{{Ordinal: 1, Value: []byte{0xde, 0xad}}}); err != nil {
			t.Fatalf("all-BLOB ExecContext failed: %v", err)
		}
		if stmtDirect.rewrittenOnServer {
			t.Fatal("expected rewrittenOnServer=false after ExecDirect all-BLOB path")
		}
		if _, err := stmtDirect.ExecContext(ctx, []driver.NamedValue{{Ordinal: 1, Value: []byte{}}}); err != nil {
			t.Fatalf("follow-up empty BLOB ExecContext failed: %v", err)
		}

		mu.Lock()
		gotDirectOps := append([]drda.CodePoint(nil), ops...)
		ops = nil
		mu.Unlock()

		if len(gotDirectOps) < 2 || gotDirectOps[0] != drda.CodePointEXCSQLIMM || gotDirectOps[1] != drda.CodePointEXCSQLSTT {
			t.Fatalf("expected [EXCSQLIMM, EXCSQLSTT, ...] without extra PRPSQLSTT, got %v", gotDirectOps)
		}

		// 2. Mixed BLOB + sql.Out INT: rewrites param 0, keeps param 1, remaps outValues[0] -> outVal
		stmtMixed := newStmt(conn, "CALL P(?, ?)", nil, []drda.ColumnDescription{
			{SQLType: uint16(codec.SQLTypeBlob), Length: 1024},
			{SQLType: uint16(codec.SQLTypeInteger), Length: 4},
		}, 2)

		var outVal int64
		mu.Lock()
		returnOutSQLDTARD = true
		ops = nil
		mu.Unlock()

		if _, err := stmtMixed.ExecContext(ctx, []driver.NamedValue{
			{Ordinal: 1, Value: []byte{0xca, 0xfe}},
			{Ordinal: 2, Value: sql.Out{Dest: &outVal}},
		}); err != nil {
			t.Fatalf("mixed BLOB + sql.Out ExecContext failed: %v", err)
		}
		if !stmtMixed.rewrittenOnServer {
			t.Fatal("expected rewrittenOnServer=true after mixed BLOB rewrite PrepareAndDescribe")
		}
		if outVal != 287449617 {
			t.Fatalf("expected remapped sql.Out value 287449617 (0x11222211), got %d", outVal)
		}

		// 3. Subsequent Batch ExecContext MUST restore original query on server BEFORE ExecBatchWithParams
		mu.Lock()
		returnOutSQLDTARD = false
		ops = nil
		mu.Unlock()

		if _, err := stmtMixed.ExecContext(ctx, []driver.NamedValue{
			{Ordinal: 1, Value: [][]byte{{1}, {2}}},
			{Ordinal: 2, Value: []int{10, 20}},
		}); err != nil {
			t.Fatalf("batch ExecContext after rewrite failed: %v", err)
		}
		if stmtMixed.rewrittenOnServer {
			t.Fatal("expected rewrittenOnServer=false after batch restore")
		}

		mu.Lock()
		gotBatchOps := append([]drda.CodePoint(nil), ops...)
		ops = nil
		mu.Unlock()

		if len(gotBatchOps) < 2 || gotBatchOps[0] != drda.CodePointPRPSQLSTT || gotBatchOps[1] != drda.CodePointEXCSQLSTT {
			t.Fatalf("expected PRPSQLSTT restore before EXCSQLSTT batch, got %v", gotBatchOps)
		}

		// 4. Subsequent QueryContext restores original query when rewrittenOnServer=true
		stmtMixed.rewrittenOnServer = true
		mu.Lock()
		ops = nil
		mu.Unlock()

		rows, err := stmtMixed.QueryContext(ctx, []driver.NamedValue{
			{Ordinal: 1, Value: []byte{}},
			{Ordinal: 2, Value: int32(9)},
		})
		if err != nil {
			t.Fatalf("QueryContext restore failed: %v", err)
		}
		_ = rows.Close()
		if stmtMixed.rewrittenOnServer {
			t.Fatal("expected rewrittenOnServer=false after QueryContext restore")
		}

		mu.Lock()
		gotQueryOps := append([]drda.CodePoint(nil), ops...)
		ops = nil
		mu.Unlock()

		if len(gotQueryOps) < 2 || gotQueryOps[0] != drda.CodePointPRPSQLSTT {
			t.Fatalf("expected PRPSQLSTT restore during QueryContext, got %v", gotQueryOps)
		}

		// 5. QueryContext closes s.isTemp if restoreOriginalQueryIfRewrittenLocked fails
		sec, err := conn.getSections().Acquire()
		if err != nil {
			t.Fatalf("section acquire failed: %v", err)
		}
		stmtTemp := newStmt(conn, "SELECT ? FROM T WHERE ID = ?", nil, []drda.ColumnDescription{
			{SQLType: uint16(codec.SQLTypeBlob), Length: 1024},
			{SQLType: uint16(codec.SQLTypeInteger), Length: 4},
		}, sec)
		stmtTemp.isTemp = true
		stmtTemp.rewrittenOnServer = true

		mu.Lock()
		failPrepare = true
		mu.Unlock()

		if _, err := stmtTemp.QueryContext(ctx, []driver.NamedValue{
			{Ordinal: 1, Value: []byte{}},
			{Ordinal: 2, Value: int32(1)},
		}); err == nil {
			t.Fatal("expected QueryContext to fail when restore PrepareAndDescribe drops")
		}
		if !stmtTemp.closed {
			t.Fatal("expected isTemp statement to be closed on restore error")
		}
	})
}

func BenchmarkDetectAndExtractBatch_Bytes(b *testing.B) {
	const batchRowsCount = 100
	intBatch := make([]int, batchRowsCount)
	byteBatch := make([][]byte, batchRowsCount)
	scalarUUID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8}
	for i := 0; i < batchRowsCount; i++ {
		intBatch[i] = i
		byteBatch[i] = []byte("payload-bytes-0123456789")
	}
	rawArgs := []any{intBatch, byteBatch, scalarUUID}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isBatch, rows, err := detectAndExtractBatch(rawArgs)
		if err != nil || !isBatch || len(rows) != batchRowsCount {
			b.Fatalf("unexpected batch result: isBatch=%v len=%d err=%v", isBatch, len(rows), err)
		}
	}
}
