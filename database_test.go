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
