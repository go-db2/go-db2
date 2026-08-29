package db2

import (
	"context"
	"database/sql"
	"testing"
)

func TestContextUserPropagation(t *testing.T) {
	ctx := context.Background()
	if user := UserFromContext(ctx); user != "" {
		t.Errorf("UserFromContext(empty) = %q, want empty string", user)
	}

	userCtx := WithUser(ctx, "tenant_alice")
	if user := UserFromContext(userCtx); user != "tenant_alice" {
		t.Errorf("UserFromContext(userCtx) = %q, want tenant_alice", user)
	}

	nilCtxUser := UserFromContext(nil)
	if nilCtxUser != "" {
		t.Errorf("UserFromContext(nil) = %q, want empty string", nilCtxUser)
	}
}

func TestSwitchUser_Validation(t *testing.T) {
	ctx := context.Background()

	// Nil target
	err := SwitchUser(ctx, nil, "alice")
	if err == nil {
		t.Error("expected error for nil target, got nil")
	}

	// Entire pool *sql.DB target
	db := &sql.DB{}
	err = SwitchUser(ctx, db, "alice")
	if err == nil {
		t.Error("expected error for *sql.DB target, got nil")
	}

	// Unsupported type
	err = SwitchUser(ctx, "invalid_target", "alice")
	if err == nil {
		t.Error("expected error for unsupported target type, got nil")
	}
}
