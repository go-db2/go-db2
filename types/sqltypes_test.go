package types

import (
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
