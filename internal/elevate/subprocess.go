package elevate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

const errSudoAskpass = "sudo askpass: %w"

// noopCleanup is returned when no temporary askpass files were created.
// Nested empty body is intentional: caller always defers cleanup safely.
func noopCleanup() {
	// no temp files to remove
}

// AttachSubprocessSudo primes sudo and configures cmd.Env so child tools (brew, mas)
// that invoke /usr/bin/sudo internally can authenticate without a TTY.
// Returns a cleanup func that removes any temporary askpass helper files.
func AttachSubprocessSudo(ctx context.Context, cmd *exec.Cmd) (func(), error) {
	if err := EnsureSudoReady(ctx); err != nil {
		return noopCleanup, err
	}

	sess := FromContext(ctx)
	if sess == nil || !sess.valid || sess.passwordless {
		return noopCleanup, nil
	}

	pwPath, err := writeSudoPasswordFile(sess.password)
	if err != nil {
		return noopCleanup, err
	}
	scriptPath, err := writeSudoAskpassScript(pwPath)
	if err != nil {
		_ = os.Remove(pwPath)
		return noopCleanup, err
	}

	cmd.Env = append(os.Environ(), "SUDO_ASKPASS="+scriptPath)
	return func() {
		_ = os.Remove(pwPath)
		_ = os.Remove(scriptPath)
	}, nil
}

func writeSudoPasswordFile(password string) (string, error) {
	pwFile, err := os.CreateTemp("", "updash-sudo-pw-*")
	if err != nil {
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	pwPath := pwFile.Name()
	if _, err := pwFile.WriteString(password + "\n"); err != nil {
		_ = pwFile.Close()
		_ = os.Remove(pwPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	if err := pwFile.Chmod(0600); err != nil {
		_ = pwFile.Close()
		_ = os.Remove(pwPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	if err := pwFile.Close(); err != nil {
		_ = os.Remove(pwPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	return pwPath, nil
}

func writeSudoAskpassScript(pwPath string) (string, error) {
	scriptFile, err := os.CreateTemp("", "updash-askpass-*")
	if err != nil {
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	scriptPath := scriptFile.Name()
	script := fmt.Sprintf("#!/bin/sh\nexec cat %q\n", pwPath)
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	if err := scriptFile.Chmod(0700); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf(errSudoAskpass, err)
	}
	return scriptPath, nil
}
