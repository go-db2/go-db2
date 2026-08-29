package db2

import (
	"context"
	"database/sql"
	"testing"
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
