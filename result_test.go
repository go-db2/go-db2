package db2

import (
	"testing"
)

func TestResultRowsAffected(t *testing.T) {
	res := NewResult(42, 0)
	affected, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if affected != 42 {
		t.Fatalf("expected affected rows 42, got %d", affected)
	}
}

func TestResultLastInsertId_Cached(t *testing.T) {
	res := NewResult(1, 999)
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 999 {
		t.Fatalf("expected LastInsertId 999, got %d", id)
	}
}

func TestResultLastInsertId_NonInsert(t *testing.T) {
	res := NewResultWithConn(nil, 5, false)
	_, err := res.LastInsertId()
	if err == nil {
		t.Fatalf("expected error for non-insert LastInsertId call, got nil")
	}
}

func TestParseIdentityValue(t *testing.T) {
	tests := []struct {
		input    any
		expected int64
	}{
		{int64(10), 10},
		{int32(20), 20},
		{int(30), 30},
		{float64(40), 40},
		{"50", 50},
		{"60.00", 60},
		{[]byte("70"), 70},
		{[]byte("80.00"), 80},
	}

	for _, tt := range tests {
		val, err := parseIdentityValue(tt.input)
		if err != nil {
			t.Fatalf("parseIdentityValue(%v) returned error: %v", tt.input, err)
		}
		if val != tt.expected {
			t.Fatalf("parseIdentityValue(%v) = %d, want %d", tt.input, val, tt.expected)
		}
	}
}
