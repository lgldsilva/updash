package updater

import (
	"context"
	"strings"
	"testing"
)

func TestExplainMasFailure_killed(t *testing.T) {
	msg := explainMasFailure("WhatsApp", "310633997", "", context.DeadlineExceeded)
	if !strings.Contains(msg, "mas interrompido") || !strings.Contains(msg, "310633997") {
		t.Fatalf("unexpected: %q", msg)
	}
}
