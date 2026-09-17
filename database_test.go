package db2

import (
	"context"
	"testing"
)

func TestCreateDb_Validation(t *testing.T) {
	tests := []struct {
		name    string
		dbname  string
		connStr string
		options []string
		wantErr bool
	}{
		{
			name:    "empty dbname",
			dbname:  "",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			wantErr: true,
		},
		{
			name:    "whitespace dbname",
			dbname:  "   ",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			wantErr: true,
		},
		{
			name:    "empty connStr",
			dbname:  "TESTDB",
			connStr: "",
			wantErr: true,
		},
		{
			name:    "invalid option format without equal sign",
			dbname:  "TESTDB",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			options: []string{"codesetUTF8"},
			wantErr: true,
		},
		{
			name:    "unsupported option key",
			dbname:  "TESTDB",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			options: []string{"invalid_opt=val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateDb(tt.dbname, tt.connStr, tt.options...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateDb() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestDropDb_Validation(t *testing.T) {
	tests := []struct {
		name    string
		dbname  string
		connStr string
		wantErr bool
	}{
		{
			name:    "empty dbname",
			dbname:  "",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			wantErr: true,
		},
		{
			name:    "whitespace dbname",
			dbname:  "   ",
			connStr: "db2://usr:pwd@localhost:50000/DB",
			wantErr: true,
		},
		{
			name:    "empty connStr",
			dbname:  "TESTDB",
			connStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DropDb(tt.dbname, tt.connStr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DropDb() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecAdminCmd_Validation(t *testing.T) {
	_, err := ExecAdminCmd(context.Background(), nil, "")
	if err == nil {
		t.Fatal("ExecAdminCmd() with empty command should return error")
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "MYDB", expected: `"MYDB"`},
		{input: "test_db", expected: `"test_db"`},
		{input: `db"name`, expected: `"db""name"`},
		{input: "", expected: `""`},
	}

	for _, tt := range tests {
		got := quoteIdentifier(tt.input)
		if got != tt.expected {
			t.Errorf("quoteIdentifier(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildCreateDbSQL(t *testing.T) {
	tests := []struct {
		name      string
		dbname    string
		codeset   string
		territory string
		mode      string
		expected  string
	}{
		{
			name:     "dbname only",
			dbname:   "TESTDB",
			expected: `CREATE DATABASE "TESTDB"`,
		},
		{
			name:     "with codeset",
			dbname:   "TESTDB",
			codeset:  "UTF-8",
			expected: `CREATE DATABASE "TESTDB" CODESET "UTF-8"`,
		},
		{
			name:      "with codeset, territory, mode",
			dbname:    "TESTDB",
			codeset:   "UTF-8",
			territory: "US",
			mode:      "RESTRICTIVE",
			expected:  `CREATE DATABASE "TESTDB" CODESET "UTF-8" TERRITORY "US" "RESTRICTIVE"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCreateDbSQL(tt.dbname, tt.codeset, tt.territory, tt.mode)
			if got != tt.expected {
				t.Errorf("buildCreateDbSQL() = %q; want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildDropDbSQL(t *testing.T) {
	got := buildDropDbSQL("TESTDB")
	expected := `DROP DATABASE "TESTDB"`
	if got != expected {
		t.Errorf("buildDropDbSQL() = %q; want %q", got, expected)
	}
}
