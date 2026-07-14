package elevate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ctxKey struct{}

// Session holds reusable elevation credentials for one updash run.
// Password bytes are kept in memory only for the session lifetime.
type Session struct {
	password     string
	valid        bool
	passwordless bool
}

// NewSession creates an empty elevation session.
func NewSession() *Session {
	return &Session{}
}

// WithSession attaches a session to ctx for updater/cleaner subprocesses.
func WithSession(ctx context.Context, s *Session) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext returns the session stored in ctx, or nil.
func FromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}

// Ready reports whether elevation is available without prompting again.
func (s *Session) Ready() bool {
	if s == nil {
		return false
	}
	return s.valid || s.passwordless
}

// SetPasswordless marks the session as using cached OS credentials (NOPASSWD / recent sudo).
func (s *Session) SetPasswordless() {
	if s == nil {
		return
	}
	s.passwordless = true
	s.valid = false
	s.password = ""
}

// Validate checks the password via `sudo -S -v` and caches it for reuse.
func (s *Session) Validate(ctx context.Context, password string) error {
	if s == nil {
		return fmt.Errorf("no elevation session")
	}
	cmd := exec.CommandContext(ctx, "sudo", "-S", "-p", "", "-v")
	cmd.Stdin = strings.NewReader(password + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "incorrect password"
		}
		return fmt.Errorf("%s", msg)
	}
	s.password = password
	s.valid = true
	s.passwordless = false
	return nil
}

// Refresh extends the sudo timestamp using the cached password.
// Call before long elevated batches so credentials do not expire mid-run.
func (s *Session) Refresh(ctx context.Context) error {
	if s == nil || !s.valid {
		return nil
	}
	cmd := exec.CommandContext(ctx, "sudo", "-S", "-p", "", "-v")
	cmd.Stdin = strings.NewReader(s.password + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "sudo session expired"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// Clear wipes cached credentials from memory.
func (s *Session) Clear() {
	if s == nil {
		return
	}
	s.password = ""
	s.valid = false
	s.passwordless = false
}

// EnsureSudoReady verifies that sudo credentials are valid for commands like mas
// that invoke /usr/bin/sudo internally. It refreshes a cached session when needed.
func EnsureSudoReady(ctx context.Context) error {
	if CanElevateWithoutPassword(ctx) {
		return nil
	}
	sess := FromContext(ctx)
	if sess == nil || !sess.Ready() {
		return fmt.Errorf("sudo password required")
	}
	if sess.passwordless {
		return nil
	}
	return sess.Refresh(ctx)
}

// CanElevateWithoutPassword checks whether sudo works without a password prompt.
func CanElevateWithoutPassword(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	return cmd.Run() == nil
}
