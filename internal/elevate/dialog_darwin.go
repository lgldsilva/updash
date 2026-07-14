//go:build darwin

package elevate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PromptMacPassword shows a native macOS dialog and returns the entered password.
func PromptMacPassword(reason string) (string, error) {
	if reason == "" {
		reason = "updash precisa da sua senha de administrador do Mac"
	}
	// Escape double quotes for AppleScript string literal.
	safe := strings.ReplaceAll(reason, `"`, `\"`)
	script := fmt.Sprintf(
		`display dialog "%s" default answer "" with hidden answer with title "updash" buttons {"Cancel", "OK"} default button "OK"`,
		safe,
	)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		if strings.Contains(err.Error(), "User canceled") || strings.Contains(string(out), "button returned:Cancel") {
			return "", ErrDialogCancelled
		}
		return "", fmt.Errorf("password dialog: %w", err)
	}
	return parseDialogPassword(string(out))
}

func parseDialogPassword(out string) (string, error) {
	text := strings.TrimSpace(out)
	if strings.Contains(text, "button returned:Cancel") {
		return "", ErrDialogCancelled
	}
	const marker = "text returned:"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return "", fmt.Errorf("password dialog: unexpected response %q", text)
	}
	return strings.TrimSpace(text[idx+len(marker):]), nil
}

// PromptMacPasswordSession prompts, validates via sudo, and returns a ready session.
func PromptMacPasswordSession(ctx context.Context, reason string) (*Session, error) {
	pw, err := PromptMacPassword(reason)
	if err != nil {
		return nil, err
	}
	sess := NewSession()
	if err := sess.Validate(ctx, pw); err != nil {
		return nil, err
	}
	return sess, nil
}
