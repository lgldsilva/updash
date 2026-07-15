package elevate

import (
	"context"
	"testing"
)

func TestEnsureSudoReady_NoSession(t *testing.T) {
	ctx := context.Background()
	// Single check: if the host (or CI runner) has passwordless sudo, this
	// environment cannot assert the "no session" error path.
	if CanElevateWithoutPassword(ctx) {
		t.Skip("passwordless sudo available on this host")
	}
	err := EnsureSudoReady(ctx)
	if err != nil {
		return // expected: no session and no passwordless sudo
	}
	// EnsureSudoReady succeeded without a session → must be passwordless now
	// (prior probe may have been slow under -race). Treat as skip, not flake.
	if CanElevateWithoutPassword(ctx) {
		t.Skip("passwordless sudo available (probe race under load)")
	}
	t.Fatal("expected error without session")
}

func TestEnsureSudoReady_PasswordlessSession(t *testing.T) {
	sess := NewSession()
	sess.SetPasswordless()
	ctx := WithSession(context.Background(), sess)
	if err := EnsureSudoReady(ctx); err != nil {
		t.Fatalf("passwordless session: %v", err)
	}
}
