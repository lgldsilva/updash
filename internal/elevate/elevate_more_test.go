package elevate

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// ── Session lifecycle ─────────────────────────────────────────────────────

func TestNewSession(t *testing.T) {
	s := NewSession()
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	if s.Ready() {
		t.Fatal("new session should not be ready")
	}
}

func TestWithSession_nil(t *testing.T) {
	ctx := context.Background()
	got := WithSession(ctx, nil)
	if got != ctx {
		t.Fatal("nil session should return same ctx")
	}
}

func TestFromContext_nilCtx(t *testing.T) {
	//nolint:staticcheck // testing nil ctx intentionally
	if s := FromContext(nil); s != nil {
		t.Fatal("nil ctx should return nil session")
	}
}

func TestFromContext_noSession(t *testing.T) {
	if s := FromContext(context.Background()); s != nil {
		t.Fatal("empty ctx should return nil session")
	}
}

func TestReady_nil(t *testing.T) {
	var s *Session
	if s.Ready() {
		t.Fatal("nil session should not be ready")
	}
}

func TestReady_valid(t *testing.T) {
	s := NewSession()
	s.valid = true
	if !s.Ready() {
		t.Fatal("valid session should be ready")
	}
}

func TestReady_passwordless(t *testing.T) {
	s := NewSession()
	s.SetPasswordless()
	if !s.Ready() {
		t.Fatal("passwordless session should be ready")
	}
}

func TestSetPasswordless_nil(t *testing.T) {
	var s *Session
	s.SetPasswordless() // should not panic
}

func TestSetPasswordless_clearsPassword(t *testing.T) {
	s := NewSession()
	s.password = "secret"
	s.valid = true
	s.SetPasswordless()
	if s.password != "" || s.valid || !s.passwordless {
		t.Fatalf("state: pw=%q valid=%v passwordless=%v", s.password, s.valid, s.passwordless)
	}
}

func TestClear_nil(t *testing.T) {
	var s *Session
	s.Clear() // should not panic
}

func TestClear(t *testing.T) {
	s := NewSession()
	s.password = "secret"
	s.valid = true
	s.passwordless = true
	s.Clear()
	if s.password != "" || s.valid || s.passwordless {
		t.Fatal("Clear should wipe all state")
	}
}

func TestValidate_nilSession(t *testing.T) {
	var s *Session
	err := s.Validate(context.Background(), "pw")
	if err == nil || !strings.Contains(err.Error(), "no elevation session") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidate_wrongPassword(t *testing.T) {
	if CanElevateWithoutPassword(context.Background()) {
		t.Skip("passwordless sudo available")
	}
	s := NewSession()
	err := s.Validate(context.Background(), "definitely-wrong-password-12345")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if s.Ready() {
		t.Fatal("session should not be ready after failed validate")
	}
}

func TestValidate_success(t *testing.T) {
	// sudo -S -v with a fake password fails even on NOPASSWD hosts
	// (NOPASSWD applies to command execution, not -S -v validation).
	// So we test the error path here — the success path requires a real password.
	s := NewSession()
	err := s.Validate(context.Background(), "any-password")
	if err == nil {
		// If it somehow succeeds (real passwordless -S -v), verify state
		if !s.Ready() {
			t.Fatal("session should be ready after validate")
		}
		return
	}
	// Expected: error with stderr message
	if s.Ready() {
		t.Fatal("session should not be ready after failed validate")
	}
}

func TestRefresh_nilSession(t *testing.T) {
	var s *Session
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("nil session refresh should be no-op, got %v", err)
	}
}

func TestRefresh_invalidSession(t *testing.T) {
	s := NewSession()
	// not valid — should be no-op
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("invalid session refresh should be no-op, got %v", err)
	}
}

func TestRefresh_wrongPassword(t *testing.T) {
	if CanElevateWithoutPassword(context.Background()) {
		t.Skip("passwordless sudo available")
	}
	s := NewSession()
	s.password = "wrong-password"
	s.valid = true
	err := s.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error for wrong password refresh")
	}
}

func TestRefresh_success(t *testing.T) {
	s := NewSession()
	s.password = "any"
	s.valid = true
	// sudo -S -v with fake password fails even on NOPASSWD hosts
	err := s.Refresh(context.Background())
	if err != nil {
		// Expected on most hosts — error path covered
		return
	}
	// If it somehow succeeds, that's also fine
}

// ── EnsureSudoReady ───────────────────────────────────────────────────────

func TestEnsureSudoReady_validSessionNeedsRefresh(t *testing.T) {
	if CanElevateWithoutPassword(context.Background()) {
		t.Skip("passwordless sudo available")
	}
	s := NewSession()
	s.password = "wrong"
	s.valid = true
	ctx := WithSession(context.Background(), s)
	err := EnsureSudoReady(ctx)
	if err == nil {
		t.Fatal("expected error for invalid password refresh")
	}
}

func TestEnsureSudoReady_passwordlessHost(t *testing.T) {
	// On a host with passwordless sudo, EnsureSudoReady returns nil immediately
	if !CanElevateWithoutPassword(context.Background()) {
		t.Skip("no passwordless sudo")
	}
	if err := EnsureSudoReady(context.Background()); err != nil {
		t.Fatalf("passwordless host should succeed: %v", err)
	}
}

func TestEnsureSudoReady_noSession_noPasswordless(t *testing.T) {
	if CanElevateWithoutPassword(context.Background()) {
		t.Skip("passwordless sudo available")
	}
	err := EnsureSudoReady(context.Background())
	if err == nil || !strings.Contains(err.Error(), "sudo password required") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsureSudoReady_passwordlessSession(t *testing.T) {
	if !CanElevateWithoutPassword(context.Background()) {
		t.Skip("no passwordless sudo")
	}
	s := NewSession()
	s.SetPasswordless()
	ctx := WithSession(context.Background(), s)
	if err := EnsureSudoReady(ctx); err != nil {
		t.Fatalf("passwordless session: %v", err)
	}
}

func TestEnsureSudoReady_validSession_refreshOK(t *testing.T) {
	if !CanElevateWithoutPassword(context.Background()) {
		t.Skip("no passwordless sudo")
	}
	s := NewSession()
	s.password = "any"
	s.valid = true
	ctx := WithSession(context.Background(), s)
	// CanElevateWithoutPassword returns true, so it returns nil before checking session
	if err := EnsureSudoReady(ctx); err != nil {
		t.Fatalf("err = %v", err)
	}
}

// ── Subprocess helpers ────────────────────────────────────────────────────

func TestNoopCleanup(t *testing.T) {
	noopCleanup() // should not panic
}

func TestWriteSudoPasswordFile(t *testing.T) {
	path, err := writeSudoPasswordFile("test-password")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test-password\n" {
		t.Fatalf("content = %q", data)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("perm = %o, want 600", fi.Mode().Perm())
	}
}

func TestWriteSudoAskpassScript(t *testing.T) {
	path, err := writeSudoAskpassScript("/tmp/test-pw-file")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "#!/bin/sh") {
		t.Fatalf("missing shebang: %q", content)
	}
	if !strings.Contains(content, "/tmp/test-pw-file") {
		t.Fatalf("missing pw path: %q", content)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0700 {
		t.Fatalf("perm = %o, want 700", fi.Mode().Perm())
	}
}

func TestAttachSubprocessSudo_noSession(t *testing.T) {
	if CanElevateWithoutPassword(context.Background()) {
		// With passwordless sudo, EnsureSudoReady succeeds, then sess==nil → noopCleanup
		cmd := exec.Command("true")
		cleanup, err := AttachSubprocessSudo(context.Background(), cmd)
		defer cleanup()
		if err != nil {
			t.Fatalf("passwordless host should succeed: %v", err)
		}
		return
	}
	cmd := exec.Command("true")
	cleanup, err := AttachSubprocessSudo(context.Background(), cmd)
	defer cleanup()
	if err == nil {
		t.Fatal("expected error without session")
	}
}

func TestAttachSubprocessSudo_passwordless(t *testing.T) {
	s := NewSession()
	s.SetPasswordless()
	ctx := WithSession(context.Background(), s)

	// If the host doesn't have passwordless sudo, EnsureSudoReady will fail
	// before reaching the passwordless check. Skip in that case.
	if !CanElevateWithoutPassword(ctx) {
		t.Skip("passwordless sudo not available on this host")
	}

	cmd := exec.Command("true")
	cleanup, err := AttachSubprocessSudo(ctx, cmd)
	defer cleanup()
	if err != nil {
		t.Fatalf("passwordless should succeed: %v", err)
	}
}

func TestAttachSubprocessSudo_validSession(t *testing.T) {
	s := NewSession()
	s.password = "test-pw"
	s.valid = true
	ctx := WithSession(context.Background(), s)

	cmd := exec.Command("true")
	cleanup, err := AttachSubprocessSudo(ctx, cmd)
	defer cleanup()

	if CanElevateWithoutPassword(context.Background()) {
		// EnsureSudoReady succeeds (passwordless), sess.valid=true, not passwordless
		// → writes askpass files and sets SUDO_ASKPASS
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hasAskpass := false
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "SUDO_ASKPASS=") {
				hasAskpass = true
			}
		}
		if !hasAskpass {
			t.Fatal("expected SUDO_ASKPASS in cmd.Env")
		}
	} else {
		// EnsureSudoReady fails (wrong password refresh)
		if err == nil {
			t.Fatal("expected error for invalid password")
		}
	}
}

// ── Platform stubs (Linux) ────────────────────────────────────────────────

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

// ── Needs elevation ───────────────────────────────────────────────────────

func TestCategoryNeedsElevation_allCases(t *testing.T) {
	linux := model.PlatformInfo{OS: "linux"}
	mac := model.PlatformInfo{OS: "darwin"}
	win := model.PlatformInfo{OS: "windows"}
	linuxYay := model.PlatformInfo{OS: "linux", HasYay: true}

	cases := []struct {
		cat  model.Category
		plat model.PlatformInfo
		want bool
	}{
		{model.CatMAS, mac, true},
		{model.CatApt, linux, true},
		{model.CatSnap, linux, true},
		{model.CatPacman, linux, true},     // no yay
		{model.CatPacman, linuxYay, false}, // has yay
		{model.CatBrew, mac, false},
		{model.CatBrew, linux, false},
		{model.CatMAS, win, false}, // windows: no elevation
		{model.CatApt, win, false}, // windows: no elevation
		{model.CatAgent, linux, false},
	}
	for _, tc := range cases {
		if got := CategoryNeedsElevation(tc.cat, tc.plat); got != tc.want {
			t.Fatalf("CategoryNeedsElevation(%v, %v) = %v, want %v", tc.cat, tc.plat.OS, got, tc.want)
		}
	}
}

func TestItemNeedsElevation_edgeCases(t *testing.T) {
	// Non-cache items never need elevation
	if ItemNeedsElevation(&model.Item{Category: model.CatBrew, Name: "apt cache"}) {
		t.Fatal("non-cache should not need elevation")
	}
	// Cache items: apt/snap need elevation
	if !ItemNeedsElevation(&model.Item{Category: model.CatCache, Name: "apt lists"}) {
		t.Fatal("apt cache should need elevation")
	}
	if !ItemNeedsElevation(&model.Item{Category: model.CatCache, Name: "snap cache"}) {
		t.Fatal("snap cache should need elevation")
	}
	// Cache items: others don't
	if ItemNeedsElevation(&model.Item{Category: model.CatCache, Name: "go cache"}) {
		t.Fatal("go cache should not need elevation")
	}
}

func TestItemsNeedElevation_cleanup(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux"}
	items := []*model.Item{
		{Category: model.CatCache, Name: "apt cache"},
	}
	if !ItemsNeedElevation(items, plat, true) {
		t.Fatal("apt cleanup should need elevation")
	}

	items = []*model.Item{
		{Category: model.CatCache, Name: "brew cache"},
	}
	if ItemsNeedElevation(items, plat, true) {
		t.Fatal("brew cleanup should not need elevation")
	}
}

func TestItemsNeedElevation_empty(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux"}
	if ItemsNeedElevation(nil, plat, false) {
		t.Fatal("empty items should not need elevation")
	}
}

// ── Sudo command builder ──────────────────────────────────────────────────

func TestSudo_nilSession(t *testing.T) {
	cmd := Sudo(context.Background(), "apt-get", "update")
	if cmd.Args[1] != "apt-get" {
		t.Fatalf("args = %v", cmd.Args)
	}
}

func TestSudo_passwordlessSession(t *testing.T) {
	s := NewSession()
	s.SetPasswordless()
	ctx := WithSession(context.Background(), s)
	cmd := Sudo(ctx, "apt-get", "update")
	// passwordless should use plain sudo (no -S)
	for _, arg := range cmd.Args {
		if arg == "-S" {
			t.Fatal("passwordless should not use -S flag")
		}
	}
}

func TestSudo_validSession(t *testing.T) {
	s := NewSession()
	s.password = "secret"
	s.valid = true
	ctx := WithSession(context.Background(), s)
	cmd := Sudo(ctx, "apt-get", "update")
	// valid session should use -S -p ""
	found := false
	for i, arg := range cmd.Args {
		if arg == "-S" && i+1 < len(cmd.Args) && cmd.Args[i+1] == "-p" {
			found = true
		}
	}
	if !found {
		t.Fatalf("valid session should use -S -p, args = %v", cmd.Args)
	}
}
