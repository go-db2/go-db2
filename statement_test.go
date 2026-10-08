package db2

import (
	"reflect"
	"testing"

	"github.com/go-db2/go-db2/network"
	"github.com/go-db2/go-db2/types"
)

func TestHasBlobParams(t *testing.T) {
	tests := []struct {
		name      string
		paramCols []network.ColumnDescription
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
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeVarChar)},
			},
			args: []any{
				[]byte("hello"),
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-empty []byte",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				[]byte{0x01, 0x02, 0x03},
			},
			want: true,
		},
		{
			name: "SQLTypeNBlob with non-empty []byte",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeNBlob)},
			},
			args: []any{
				[]byte{0xDE, 0xAD, 0xBE, 0xEF},
			},
			want: true,
		},
		{
			name: "SQLTypeBlob with empty []byte",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				[]byte{},
			},
			want: false,
		},
		{
			name: "SQLTypeNBlob with empty []byte",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeNBlob)},
			},
			args: []any{
				[]byte{},
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-[]byte argument (string)",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				"not bytes",
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with non-[]byte argument (int)",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				12345,
			},
			want: false,
		},
		{
			name: "SQLTypeBlob with nil argument",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				nil,
			},
			want: false,
		},
		{
			name: "Bounds check: paramCols longer than args where BLOB is out of bounds",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger)},
				{SQLType: uint16(types.SQLTypeBlob)},
			},
			args: []any{
				10,
			},
			want: false,
		},
		{
			name: "Multiple parameters where BLOB parameter is at index > 0",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger)},
				{SQLType: uint16(types.SQLTypeVarChar)},
				{SQLType: uint16(types.SQLTypeBlob)},
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
		paramCols         []network.ColumnDescription
		args              []any
		wantQuery         string
		wantRewrittenCols []network.ColumnDescription
		wantRewrittenArgs []any
	}{
		{
			name:              "No parameters",
			query:             "SELECT * FROM users WHERE id = 1",
			paramCols:         []network.ColumnDescription{},
			args:              []any{},
			wantQuery:         "SELECT * FROM users WHERE id = 1",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Non-BLOB parameters only",
			query: "INSERT INTO users (id, name) VALUES (?, ?)",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(types.SQLTypeVarChar), Name: "NAME"},
			},
			args:      []any{1, "Alice"},
			wantQuery: "INSERT INTO users (id, name) VALUES (?, ?)",
			wantRewrittenCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(types.SQLTypeVarChar), Name: "NAME"},
			},
			wantRewrittenArgs: []any{1, "Alice"},
		},
		{
			name:  "BLOB parameter rewritten to BLOB(X'...')",
			query: "INSERT INTO files (id, data) VALUES (?, ?)",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:      []any{42, []byte{0xAB, 0xCD, 0xEF}},
			wantQuery: "INSERT INTO files (id, data) VALUES (?, BLOB(X'abcdef'))",
			wantRewrittenCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
			},
			wantRewrittenArgs: []any{42},
		},
		{
			name:  "SQLTypeNBlob parameter rewritten to BLOB(X'...')",
			query: "UPDATE files SET data = ? WHERE id = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeNBlob), Name: "DATA"},
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
			},
			args:      []any{[]byte{0x01, 0x02}, 10},
			wantQuery: "UPDATE files SET data = BLOB(X'0102') WHERE id = ?",
			wantRewrittenCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
			},
			wantRewrittenArgs: []any{10},
		},
		{
			name:  "BLOB with empty []byte is kept as parameter",
			query: "INSERT INTO files (data) VALUES (?)",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:      []any{[]byte{}},
			wantQuery: "INSERT INTO files (data) VALUES (?)",
			wantRewrittenCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			wantRewrittenArgs: []any{[]byte{}},
		},
		{
			name:  "Ignore ? in single-quoted string literal",
			query: "SELECT * FROM t WHERE note = 'Is this ? real?' AND data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x12}},
			wantQuery:         "SELECT * FROM t WHERE note = 'Is this ? real?' AND data = BLOB(X'12')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in escaped single-quoted string literal",
			query: "SELECT * FROM t WHERE note = 'It''s a ? test' AND data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x12}},
			wantQuery:         "SELECT * FROM t WHERE note = 'It''s a ? test' AND data = BLOB(X'12')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in double-quoted identifier",
			query: "SELECT * FROM \"col?name\" WHERE data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x34}},
			wantQuery:         "SELECT * FROM \"col?name\" WHERE data = BLOB(X'34')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in escaped double-quoted identifier",
			query: "SELECT * FROM \"col\"\"?name\" WHERE data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x34}},
			wantQuery:         "SELECT * FROM \"col\"\"?name\" WHERE data = BLOB(X'34')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in single-line comment",
			query: "SELECT 1 -- Is this a question mark ? \n FROM t WHERE data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x56}},
			wantQuery:         "SELECT 1 -- Is this a question mark ? \n FROM t WHERE data = BLOB(X'56')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Ignore ? in block comment",
			query: "SELECT /* ? comment ? */ * FROM t WHERE data = ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeBlob), Name: "DATA"},
			},
			args:              []any{[]byte{0x78}},
			wantQuery:         "SELECT /* ? comment ? */ * FROM t WHERE data = BLOB(X'78')",
			wantRewrittenCols: nil,
			wantRewrittenArgs: nil,
		},
		{
			name:  "Extra ? in query beyond paramCols length",
			query: "SELECT ?, ?",
			paramCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
			},
			args:      []any{1},
			wantQuery: "SELECT ?, ?",
			wantRewrittenCols: []network.ColumnDescription{
				{SQLType: uint16(types.SQLTypeInteger), Name: "ID"},
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
