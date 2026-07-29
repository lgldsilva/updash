//go:build !darwin

package elevate

import (
	"context"
	"testing"
)

func TestPromptMacPassword_other(t *testing.T) {
	_, err := PromptMacPassword("test")
	if err != ErrDialogUnavailable {
		t.Fatalf("err = %v, want ErrDialogUnavailable", err)
	}
}

func TestPromptMacPasswordSession_other(t *testing.T) {
	_, err := PromptMacPasswordSession(context.Background(), "test")
	if err != ErrDialogUnavailable {
		t.Fatalf("err = %v, want ErrDialogUnavailable", err)
	}
}

func TestNativeMacAuthAvailable_other(t *testing.T) {
	if NativeMacAuthAvailable() {
		t.Fatal("should be false on non-darwin")
	}
}

func TestPrimeMacOSUserSudo_other(t *testing.T) {
	err := PrimeMacOSUserSudo(context.Background())
	if err != ErrDialogUnavailable {
		t.Fatalf("err = %v, want ErrDialogUnavailable", err)
	}
}

func TestRunPrivilegedScript_other(t *testing.T) {
	_, err := RunPrivilegedScript(context.Background(), "echo hi")
	if err != ErrDialogUnavailable {
		t.Fatalf("err = %v, want ErrDialogUnavailable", err)
	}
}
