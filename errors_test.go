package db2_test

import (
	"errors"
	"testing"

	db2 "github.com/go-db2/go-db2"
	"github.com/go-db2/go-db2/internal/drda"
)

func TestStructuredError_As(t *testing.T) {
	var err error = drda.NewError(-204, "42704", "undefined table name", nil)

	var dbErr *db2.Error
	if !errors.As(err, &dbErr) {
		t.Fatalf("expected errors.As to match *db2.Error, got false")
	}

	if dbErr.SQLCode != -204 {
		t.Errorf("expected SQLCode -204, got %d", dbErr.SQLCode)
	}
	if dbErr.SQLState != "42704" {
		t.Errorf("expected SQLState 42704, got %s", dbErr.SQLState)
	}
	if expected := "db2: SQLCODE=-204, SQLSTATE=42704: undefined table name"; dbErr.Error() != expected {
		t.Errorf("expected %q, got %q", expected, dbErr.Error())
	}

	// Backwards compatibility alias Db2Error
	var legacyErr *db2.Db2Error
	if !errors.As(err, &legacyErr) {
		t.Fatalf("expected errors.As to match *db2.Db2Error alias, got false")
	}
}

func TestStructuredError_IsSentinels(t *testing.T) {
	authErr := drda.NewError(-30082, "08001", "SECCHKRM security check failed", drda.ErrAuthenticationFailed)
	if !errors.Is(authErr, db2.ErrAuthenticationFailed) {
		t.Errorf("expected errors.Is(authErr, db2.ErrAuthenticationFailed) to be true")
	}

	rdbErr := drda.NewError(-1013, "42705", "relational database not found", drda.ErrDatabaseNotFound)
	if !errors.Is(rdbErr, db2.ErrDatabaseNotFound) {
		t.Errorf("expected errors.Is(rdbErr, db2.ErrDatabaseNotFound) to be true")
	}
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ErrConnectionClosed",
			err:      db2.ErrConnectionClosed,
			expected: "db2: connection is closed",
		},
		{
			name:     "ErrDatabaseNotFound",
			err:      db2.ErrDatabaseNotFound,
			expected: "db2: relational database not found (RDBNFNRM)",
		},
		{
			name:     "ErrAuthenticationFailed",
			err:      db2.ErrAuthenticationFailed,
			expected: "db2: authentication failed (SECCHKRM)",
		},
		{
			name:     "ErrInvalidConnectionStr",
			err:      db2.ErrInvalidConnectionStr,
			expected: "db2: invalid connection string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("%s is nil", tt.name)
			}
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("errors.Is(%v, %v) = false, want true", tt.err, tt.err)
			}
		})
	}
}
