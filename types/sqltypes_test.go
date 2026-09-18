package types

import (
	"math"
	"testing"
)

func TestSQLType(t *testing.T) {
	tests := []struct {
		sqlType    SQLType
		isNullable bool
		baseType   SQLType
		expected   string
	}{
		{SQLTypeInteger, false, SQLTypeInteger, "INTEGER"},
		{SQLTypeNInteger, true, SQLTypeInteger, "INTEGER (NULLABLE)"},
		{SQLTypeVarChar, false, SQLTypeVarChar, "VARCHAR"},
		{SQLTypeNVarChar, true, SQLTypeVarChar, "VARCHAR (NULLABLE)"},
		{SQLTypeTimestamp, false, SQLTypeTimestamp, "TIMESTAMP"},
		{SQLTypeNTimestamp, true, SQLTypeTimestamp, "TIMESTAMP (NULLABLE)"},
		{SQLTypeBlob, false, SQLTypeBlob, "BLOB"},
		{SQLTypeNBlob, true, SQLTypeBlob, "BLOB (NULLABLE)"},
		{SQLType(9999), true, SQLType(9998), "SQLType(9999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.sqlType.IsNullable(); got != tt.isNullable {
				t.Errorf("SQLType.IsNullable() = %v, want %v", got, tt.isNullable)
			}
			if got := tt.sqlType.BaseType(); got != tt.baseType {
				t.Errorf("SQLType.BaseType() = %v, want %v", got, tt.baseType)
			}
			if got := tt.sqlType.String(); got != tt.expected {
				t.Errorf("SQLType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSQLType_IsNullable(t *testing.T) {
	tests := []struct {
		name       string
		sqlType    SQLType
		isNullable bool
	}{
		{"Integer Non-Nullable", SQLTypeInteger, false},
		{"Integer Nullable", SQLTypeNInteger, true},
		{"VarChar Non-Nullable", SQLTypeVarChar, false},
		{"VarChar Nullable", SQLTypeNVarChar, true},
		{"DecFloat Non-Nullable", SQLTypeDecFloat, false},
		{"DecFloat Nullable", SQLTypeNDecFloat, true},
		{"Boolean Non-Nullable", SQLTypeBoolean, false},
		{"Boolean Nullable", SQLTypeNBoolean, true},
		{"Zero (even)", SQLType(0), false},
		{"Arbitrary Even (1000)", SQLType(1000), false},
		{"Arbitrary Odd (1001)", SQLType(1001), true},
		{"MaxUint16 (65535, odd)", SQLType(math.MaxUint16), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sqlType.IsNullable(); got != tt.isNullable {
				t.Errorf("SQLType(%d).IsNullable() = %v, want %v", tt.sqlType, got, tt.isNullable)
			}
		})
	}
}
