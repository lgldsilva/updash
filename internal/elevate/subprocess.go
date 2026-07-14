package elevate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// AttachSubprocessSudo primes sudo and configures cmd.Env so child tools (brew, mas)
// that invoke /usr/bin/sudo internally can authenticate without a TTY.
// Returns a cleanup func that removes any temporary askpass helper files.
func AttachSubprocessSudo(ctx context.Context, cmd *exec.Cmd) (func(), error) {
	if err := EnsureSudoReady(ctx); err != nil {
		return func() {}, err
	}

	sess := FromContext(ctx)
	if sess == nil || !sess.valid || sess.passwordless {
		return func() {}, nil
	}

	pwFile, err := os.CreateTemp("", "updash-sudo-pw-*")
	if err != nil {
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	pwPath := pwFile.Name()
	if _, err := pwFile.WriteString(sess.password + "\n"); err != nil {
		_ = pwFile.Close()
		_ = os.Remove(pwPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	if err := pwFile.Chmod(0600); err != nil {
		_ = pwFile.Close()
		_ = os.Remove(pwPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	if err := pwFile.Close(); err != nil {
		_ = os.Remove(pwPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}

	scriptFile, err := os.CreateTemp("", "updash-askpass-*")
	if err != nil {
		_ = os.Remove(pwPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	scriptPath := scriptFile.Name()
	script := fmt.Sprintf("#!/bin/sh\nexec cat %q\n", pwPath)
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(pwPath)
		_ = os.Remove(scriptPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	if err := scriptFile.Chmod(0700); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(pwPath)
		_ = os.Remove(scriptPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(pwPath)
		_ = os.Remove(scriptPath)
		return func() {}, fmt.Errorf("sudo askpass: %w", err)
	}

	cmd.Env = append(os.Environ(), "SUDO_ASKPASS="+scriptPath)

	cleanup := func() {
		_ = os.Remove(pwPath)
		_ = os.Remove(scriptPath)
	}
	return cleanup, nil
}
