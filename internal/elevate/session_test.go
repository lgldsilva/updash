package elevate

import (
	"context"
	"testing"
)

func TestEnsureSudoReady_NoSession(t *testing.T) {
	ctx := context.Background()
	if err := EnsureSudoReady(ctx); err == nil {
		t.Fatal("expected error without session")
	}
}

func TestEnsureSudoReady_PasswordlessSession(t *testing.T) {
	sess := NewSession()
	sess.SetPasswordless()
	ctx := WithSession(context.Background(), sess)
	if err := EnsureSudoReady(ctx); err != nil {
		t.Fatalf("passwordless session: %v", err)
	}
}
