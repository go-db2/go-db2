package db2

import (
	"context"
	"database/sql"
	"testing"

	"github.com/go-db2/go-db2/network"
)

func TestClientInfoContext(t *testing.T) {
	ctx := context.Background()
	emptyInfo := ClientInfoFromContext(ctx)
	if !emptyInfo.IsEmpty() {
		t.Errorf("expected empty ClientInfo, got %+v", emptyInfo)
	}

	info := ClientInfo{
		ApplicationName:  "billing-service",
		WorkstationName:  "pod-worker-01",
		UserID:           "user_456",
		Accounting:       "cost_center_fin",
		CorrelationToken: "trace-abc-123",
	}

	if info.IsEmpty() {
		t.Error("expected IsEmpty() == false")
	}

	clientCtx := WithClientInfo(ctx, info)
	extracted := ClientInfoFromContext(clientCtx)

	if extracted.ApplicationName != info.ApplicationName {
		t.Errorf("ApplicationName = %q, want %q", extracted.ApplicationName, info.ApplicationName)
	}
	if extracted.WorkstationName != info.WorkstationName {
		t.Errorf("WorkstationName = %q, want %q", extracted.WorkstationName, info.WorkstationName)
	}
	if extracted.UserID != info.UserID {
		t.Errorf("UserID = %q, want %q", extracted.UserID, info.UserID)
	}
	if extracted.Accounting != info.Accounting {
		t.Errorf("Accounting = %q, want %q", extracted.Accounting, info.Accounting)
	}
	if extracted.CorrelationToken != info.CorrelationToken {
		t.Errorf("CorrelationToken = %q, want %q", extracted.CorrelationToken, info.CorrelationToken)
	}

	nilCtxInfo := ClientInfoFromContext(nil)
	if !nilCtxInfo.IsEmpty() {
		t.Errorf("expected empty ClientInfo for nil context, got %+v", nilCtxInfo)
	}
}

func TestClientInfo_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		info ClientInfo
		want bool
	}{
		{
			name: "zero value struct is empty",
			info: ClientInfo{},
			want: true,
		},
		{
			name: "only ApplicationName set is not empty",
			info: ClientInfo{ApplicationName: "app"},
			want: false,
		},
		{
			name: "only WorkstationName set is not empty",
			info: ClientInfo{WorkstationName: "ws"},
			want: false,
		},
		{
			name: "only UserID set is not empty",
			info: ClientInfo{UserID: "usr"},
			want: false,
		},
		{
			name: "only Accounting set is not empty",
			info: ClientInfo{Accounting: "acct"},
			want: false,
		},
		{
			name: "only CorrelationToken set is not empty",
			info: ClientInfo{CorrelationToken: "token"},
			want: false,
		},
		{
			name: "all fields set is not empty",
			info: ClientInfo{
				ApplicationName:  "app",
				WorkstationName:  "ws",
				UserID:           "usr",
				Accounting:       "acct",
				CorrelationToken: "token",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsEmpty(); got != tt.want {
				t.Errorf("ClientInfo.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetClientInfo_Validation(t *testing.T) {
	ctx := context.Background()

	// Nil target
	err := SetClientInfo(ctx, nil, ClientInfo{ApplicationName: "test"})
	if err == nil {
		t.Error("expected error for nil target, got nil")
	}

	// *sql.DB target
	db := &sql.DB{}
	err = SetClientInfo(ctx, db, ClientInfo{ApplicationName: "test"})
	if err == nil {
		t.Error("expected error for *sql.DB target, got nil")
	}

	// Unsupported type
	err = SetClientInfo(ctx, "invalid_target", ClientInfo{ApplicationName: "test"})
	if err == nil {
		t.Error("expected error for unsupported target type, got nil")
	}
}

func TestSetClientInfo_CorrelationToken(t *testing.T) {
	ctx := context.Background()
	sess := network.NewSession(network.SessionConfig{Database: "TESTDB"})
	conn := NewConn(sess, &Config{Database: "TESTDB"})

	info := ClientInfo{
		ApplicationName:  "audit-service",
		WorkstationName:  "node-01",
		UserID:           "auditor",
		Accounting:       "acct-dept",
		CorrelationToken: "trace-corr-999",
	}

	// SetClientInfo on closed connection returns ErrConnectionClosed
	_ = conn.Close()
	err := conn.SetClientInfo(ctx, info)
	if err == nil {
		t.Error("expected error calling SetClientInfo on closed connection, got nil")
	}
}
