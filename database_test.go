package db2

import (
	"context"
	"strings"
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

func TestValidateSQLIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		val       string
		fieldName string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "valid simple identifier",
			val:       "MY_DB1",
			fieldName: "database name",
			wantErr:   false,
		},
		{
			name:      "valid identifier with special allowed chars",
			val:       "DB_#$123",
			fieldName: "database name",
			wantErr:   false,
		},
		{
			name:      "valid identifier max length 128",
			val:       strings.Repeat("A", 128),
			fieldName: "database name",
			wantErr:   false,
		},
		{
			name:      "empty string",
			val:       "",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:      "whitespace string",
			val:       "   ",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:      "exceeds max length 128",
			val:       strings.Repeat("A", 129),
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char space",
			val:       "MY DB",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char hyphen",
			val:       "MY-DB",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char quote",
			val:       `MY"DB`,
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char at sign",
			val:       "MY@DB",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char semicolon",
			val:       "MY;DB",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "invalid char percent",
			val:       "MY%DB",
			fieldName: "database name",
			wantErr:   true,
			errSubstr: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSQLIdentifier(tt.val, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSQLIdentifier() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validateSQLIdentifier() error = %v, expected substring %q", err, tt.errSubstr)
				}
			}
		})
	}
}

func TestExecAdminCmd_Validation(t *testing.T) {
	_, err := ExecAdminCmd(context.Background(), nil, "")
	if err == nil {
		t.Fatal("ExecAdminCmd() with empty command should return error")
	}

	_, err = ExecAdminCmd(context.Background(), nil, "RUNSTATS ON TABLE USERS")
	if err == nil {
		t.Fatal("ExecAdminCmd() with nil *sql.DB should return error")
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

func BenchmarkQuoteIdentifier_NoQuotes(b *testing.B) {
	name := "TEST_DATABASE_IDENTIFIER"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = quoteIdentifier(name)
	}
}

func BenchmarkQuoteIdentifier_WithQuotes(b *testing.B) {
	name := `TEST_"DATABASE"_IDENTIFIER`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = quoteIdentifier(name)
	}
}
