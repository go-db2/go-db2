package validate

import (
	"strings"
	"testing"
)

func TestIsValidSQLIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single character",
			input:    "A",
			expected: true,
		},
		{
			name:     "standard alphanumeric and allowed symbols",
			input:    "Db2_User#123$",
			expected: true,
		},
		{
			name:     "exactly 128 characters",
			input:    strings.Repeat("A", 128),
			expected: true,
		},
		{
			name:     "exceeds max length (129 characters)",
			input:    strings.Repeat("A", 129),
			expected: false,
		},
		{
			name:     "contains hyphen",
			input:    "USER-NAME",
			expected: false,
		},
		{
			name:     "contains space",
			input:    "USER NAME",
			expected: false,
		},
		{
			name:     "contains trailing newline",
			input:    "USERNAME\n",
			expected: false,
		},
		{
			name:     "contains carriage return",
			input:    "USERNAME\r",
			expected: false,
		},
		{
			name:     "contains UTF-8 character",
			input:    "USER_NÄME",
			expected: false,
		},
		{
			name:     "contains SQL injection character quote",
			input:    "USER'NAME",
			expected: false,
		},
		{
			name:     "contains semicolon",
			input:    "USER;NAME",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidSQLIdentifier(tt.input)
			if got != tt.expected {
				t.Errorf("IsValidSQLIdentifier(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func BenchmarkIsValidSQLIdentifier_Valid(b *testing.B) {
	name := "TEST_DATABASE_123"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsValidSQLIdentifier(name)
	}
}

func BenchmarkIsValidSQLIdentifier_Invalid(b *testing.B) {
	name := "TEST_DATABASE-123!"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsValidSQLIdentifier(name)
	}
}
