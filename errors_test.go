package db2

import (
	"errors"
	"testing"
)

func TestDb2Error_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      Db2Error
		expected string
	}{
		{
			name: "With message and positive SQLCODE",
			err: Db2Error{
				SQLCode:  100,
				SQLState: "02000",
				Message:  "Row not found",
			},
			expected: "db2: SQLCODE=100 SQLSTATE=02000 Row not found",
		},
		{
			name: "With message and negative SQLCODE (-204 object not defined)",
			err: Db2Error{
				SQLCode:  -204,
				SQLState: "42704",
				Message:  "UNDEFINED NAME",
			},
			expected: "db2: SQLCODE=-204 SQLSTATE=42704 UNDEFINED NAME",
		},
		{
			name: "With message and negative SQLCODE (-803 duplicate key)",
			err: Db2Error{
				SQLCode:  -803,
				SQLState: "23505",
				Message:  "DUPLICATE KEY VALUE",
			},
			expected: "db2: SQLCODE=-803 SQLSTATE=23505 DUPLICATE KEY VALUE",
		},
		{
			name: "Without message",
			err: Db2Error{
				SQLCode:  -204,
				SQLState: "42704",
				Message:  "",
			},
			expected: "db2: SQLCODE=-204 SQLSTATE=42704",
		},
		{
			name: "Zero SQLCode without message",
			err: Db2Error{
				SQLCode:  0,
				SQLState: "00000",
				Message:  "",
			},
			expected: "db2: SQLCODE=0 SQLSTATE=00000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Db2Error.Error() = %q, want %q", got, tt.expected)
			}
		})
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
			err:      ErrConnectionClosed,
			expected: "db2: connection is closed",
		},
		{
			name:     "ErrDatabaseNotFound",
			err:      ErrDatabaseNotFound,
			expected: "db2: relational database not found (RDBNFNRM)",
		},
		{
			name:     "ErrAuthenticationFailed",
			err:      ErrAuthenticationFailed,
			expected: "db2: authentication failed (SECCHKRM)",
		},
		{
			name:     "ErrInvalidConnectionStr",
			err:      ErrInvalidConnectionStr,
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
