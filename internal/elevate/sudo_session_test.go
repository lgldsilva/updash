package elevate

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func readySession(password string) *Session {
	s := NewSession()
	s.password, s.valid = password, true
	return s
}

func TestCanElevateWithoutPassword(t *testing.T) {
	for _, nopasswd := range []bool{false, true} {
		hostSudo(t, nopasswd)
		if got := CanElevateWithoutPassword(context.Background()); got != nopasswd {
			t.Fatalf("nopasswd=%v: got %v", nopasswd, got)
		}
	}
}

func TestValidate(t *testing.T) {
	hostSudo(t, false)
	s := NewSession()
	err := s.Validate(context.Background(), "wrong")
	if err == nil || !strings.Contains(err.Error(), "Sorry, try again") || s.Ready() {
		t.Fatalf("wrong password: err=%v ready=%v", err, s.Ready())
	}
	if err := s.Validate(context.Background(), fakeSudoPassword); err != nil || !s.Ready() {
		t.Fatalf("right password: err=%v ready=%v", err, s.Ready())
	}
	if s.password != fakeSudoPassword || s.passwordless {
		t.Fatalf("session state after validate: %+v", s)
	}
}

func TestRefresh(t *testing.T) {
	hostSudo(t, false)
	if err := readySession(fakeSudoPassword).Refresh(context.Background()); err != nil {
		t.Fatalf("valid password refresh: %v", err)
	}
	if err := readySession("stale").Refresh(context.Background()); err == nil {
		t.Fatal("refresh with a wrong cached password must fail")
	}
}

func TestEnsureSudoReady(t *testing.T) {
	passwordless := NewSession()
	passwordless.SetPasswordless()
	cases := []struct {
		name     string
		nopasswd bool
		sess     *Session
		wantErr  string
	}{
		{"nopasswd host, no session", true, nil, ""},
		{"nopasswd host ignores a stale session", true, readySession("stale"), ""},
		{"no session", false, nil, "sudo password required"},
		{"passwordless session", false, passwordless, ""},
		{"valid session refreshes", false, readySession(fakeSudoPassword), ""},
		{"stale session fails refresh", false, readySession("stale"), "Sorry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostSudo(t, tc.nopasswd)
			ctx := context.Background()
			if tc.sess != nil {
				ctx = WithSession(ctx, tc.sess)
			}
			err := EnsureSudoReady(ctx)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestAttachSubprocessSudo(t *testing.T) {
	passwordless := NewSession()
	passwordless.SetPasswordless()
	cases := []struct {
		name        string
		nopasswd    bool
		sess        *Session
		wantErr     bool
		wantAskpass bool
	}{
		{"no session, password host", false, nil, true, false},
		{"no session, nopasswd host", true, nil, false, false},
		{"passwordless session", false, passwordless, false, false},
		{"valid session installs askpass", false, readySession(fakeSudoPassword), false, true},
		{"stale session", false, readySession("stale"), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostSudo(t, tc.nopasswd)
			ctx := context.Background()
			if tc.sess != nil {
				ctx = WithSession(ctx, tc.sess)
			}
			cmd := exec.Command("true")
			cleanup, err := AttachSubprocessSudo(ctx, cmd)
			defer cleanup()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			hasAskpass := false
			for _, kv := range cmd.Env {
				hasAskpass = hasAskpass || strings.HasPrefix(kv, "SUDO_ASKPASS=")
			}
			if hasAskpass != tc.wantAskpass {
				t.Fatalf("SUDO_ASKPASS set = %v, want %v", hasAskpass, tc.wantAskpass)
			}
		})
	}
}

// A command built by Sudo with a validated session must authenticate with the
// cached password through stdin — run it against the fake to prove it.
func TestSudo_ValidSessionFeedsPassword(t *testing.T) {
	hostSudo(t, false)
	ctx := WithSession(context.Background(), readySession(fakeSudoPassword))
	if err := Sudo(ctx, "apt-get", "update").Run(); err != nil {
		t.Fatalf("sudo with cached password: %v", err)
	}
	ctx = WithSession(context.Background(), readySession("stale"))
	if err := Sudo(ctx, "apt-get", "update").Run(); err == nil {
		t.Fatal("a stale cached password must be rejected")
	}
}
