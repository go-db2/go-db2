package network

import (
	"testing"
)

func TestCodePointString(t *testing.T) {
	tests := []struct {
		cp       CodePoint
		expected string
	}{
		{CodePointEXCSAT, "EXCSAT"},
		{CodePointACCSEC, "ACCSEC"},
		{CodePointSECCHK, "SECCHK"},
		{CodePointACCRDB, "ACCRDB"},
		{CodePointPRPSQLSTT, "PRPSQLSTT"},
		{CodePointEXCSQLSTT, "EXCSQLSTT"},
		{CodePointOPNQRY, "OPNQRY"},
		{CodePointCLSQRY, "CLSQRY"},
		{CodePointRDBCMM, "RDBCMM"},
		{CodePointRDBRLLBCK, "RDBRLLBCK"},
		{CodePoint(0xFFFF), "CodePoint(0xFFFF)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.cp.String(); got != tt.expected {
				t.Errorf("CodePoint.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
