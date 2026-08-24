package updater

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExplainMasFailure_killed(t *testing.T) {
	msg := explainMasFailure("WhatsApp", "310633997", "", context.DeadlineExceeded)
	if !strings.Contains(msg, "tempo limite") || !strings.Contains(msg, "310633997") {
		t.Fatalf("unexpected: %q", msg)
	}
}

func TestExplainMasFailure_SignalIsNotPasswordExpiry(t *testing.T) {
	msg := explainMasFailure("WhatsApp", "310633997", "signal: killed", errors.New("signal: killed"))
	if strings.Contains(strings.ToLower(msg), "senha") || strings.Contains(strings.ToLower(msg), "sudo expirou") {
		t.Fatalf("signal failure must not infer password expiry: %q", msg)
	}
}
