package db2

import (
	"testing"
)

func TestNewResult(t *testing.T) {
	tests := []struct {
		name         string
		affectedRows int64
		lastInsertId int64
		expectError  bool
	}{
		{
			name:         "positive values",
			affectedRows: 10,
			lastInsertId: 100,
			expectError:  false,
		},
		{
			name:         "zero values",
			affectedRows: 0,
			lastInsertId: 0,
			expectError:  true,
		},
		{
			name:         "negative values",
			affectedRows: -1,
			lastInsertId: -1,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewResult(tt.affectedRows, tt.lastInsertId)
			if res == nil {
				t.Fatalf("NewResult(%d, %d) returned nil", tt.affectedRows, tt.lastInsertId)
			}

			if res.affectedRows != tt.affectedRows {
				t.Errorf("res.affectedRows = %d, want %d", res.affectedRows, tt.affectedRows)
			}
			if res.lastInsertId != tt.lastInsertId {
				t.Errorf("res.lastInsertId = %d, want %d", res.lastInsertId, tt.lastInsertId)
			}

			affected, err := res.RowsAffected()
			if err != nil {
				t.Errorf("RowsAffected() unexpected error: %v", err)
			}
			if affected != tt.affectedRows {
				t.Errorf("RowsAffected() = %d, want %d", affected, tt.affectedRows)
			}

			id, err := res.LastInsertId()
			if tt.expectError {
				if err == nil {
					t.Errorf("LastInsertId() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("LastInsertId() unexpected error: %v", err)
				}
				if id != tt.lastInsertId {
					t.Errorf("LastInsertId() = %d, want %d", id, tt.lastInsertId)
				}
			}
		})
	}
}

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
