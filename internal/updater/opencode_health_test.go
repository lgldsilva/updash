package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// stubOpenCodeInstall pins the detected OpenCode installation for one test so
// the plan does not depend on how the machine running the suite installed it.
func stubOpenCodeInstall(t *testing.T, info scanner.OpenCodeInstallInfo) {
	t.Helper()
	prev := openCodeInstall
	openCodeInstall = func() scanner.OpenCodeInstallInfo { return info }
	t.Cleanup(func() { openCodeInstall = prev })
}

// tempExecutable creates a real executable file so os.Stat inside
// openCodeBinaryOK sees a runnable launcher (lookPath is stubbed to return it).
func tempExecutable(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	p := filepath.Join(d, "opencode")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake launcher: %v", err)
	}
	return p
}

func TestEnsureOpenCodeHealthy_Healthy(t *testing.T) {
	exe := tempExecutable(t)
	prevRunner, prevLook := outputRunner, lookPath
	outputRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("1.18.16"), nil
	}
	lookPath = func(file string) (string, error) { return exe, nil }
	t.Cleanup(func() { outputRunner, lookPath = prevRunner, prevLook })

	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent}
	res := &Result{Item: item, Success: true, Output: "upgrade output"}
	got := ensureOpenCodeHealthy(context.Background(), item, res)
	if !got.Success || item.Status != model.StatusDone {
		t.Fatalf("expected healthy/done, got success=%v status=%v err=%q", got.Success, item.Status, got.Error)
	}
}

// Update reported success but the launcher is a broken stub: must fail
// explicitly with the actionable reinstall hint (not advertise success).
func TestEnsureOpenCodeHealthy_BrokenStub(t *testing.T) {
	prevRunner, prevLook := outputRunner, lookPath
	outputRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("opencode: native binary not installed")
	}
	lookPath = func(file string) (string, error) { return "/nonexistent/opencode", nil }
	t.Cleanup(func() { outputRunner, lookPath = prevRunner, prevLook })

	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent}
	res := &Result{Item: item, Success: true} // upgrade reported success
	got := ensureOpenCodeHealthy(context.Background(), item, res)
	if got.Success {
		t.Fatal("broken opencode must not report success")
	}
	if item.Status != model.StatusError {
		t.Fatalf("status = %v, want StatusError", item.Status)
	}
	if !strings.Contains(got.Error, opencodeReinstallHint) {
		t.Errorf("error must include reinstall hint %q, got %q", opencodeReinstallHint, got.Error)
	}
}

// A failed update is preserved as-is; no health probe needed.
func TestEnsureOpenCodeHealthy_UpdateFailed(t *testing.T) {
	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent, Status: model.StatusUpdating}
	res := &Result{Item: item, Success: false, Error: "upgrade failed"}
	got := ensureOpenCodeHealthy(context.Background(), item, res)
	if got.Success || got.Error != "upgrade failed" {
		t.Fatalf("expected original failure preserved, got success=%v err=%q", got.Success, got.Error)
	}
}

// End-to-end: updateAgent("OpenCode") dispatches `opencode upgrade` (single
// owner) and then validates health, ending in StatusDone.
func TestUpdateAgent_OpenCode(t *testing.T) {
	exe := tempExecutable(t)
	var dispatched []string
	prevRun, prevRunner, prevLook := runUpdateCmd, outputRunner, lookPath
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		dispatched = append([]string{name}, args...)
		return "upgrade done", "", nil
	}
	outputRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("1.18.16"), nil
	}
	lookPath = func(file string) (string, error) { return exe, nil }
	t.Cleanup(func() { runUpdateCmd, outputRunner, lookPath = prevRun, prevRunner, prevLook })
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{Method: scanner.OpenCodeMethodCurl, BinPath: exe})

	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent, Status: model.StatusOutdated}
	res := updateAgent(context.Background(), item, SilentOptions())
	if !res.Success || item.Status != model.StatusDone {
		t.Fatalf("expected success/done, got success=%v status=%v err=%q", res.Success, item.Status, res.Error)
	}
	want := []string{"opencode", "upgrade", "-m", scanner.OpenCodeMethodCurl}
	if !slices.Equal(dispatched, want) {
		t.Fatalf("dispatched = %v, want %v", dispatched, want)
	}
}

func TestExecutePreparedBatch_OpenCodeKeepsHealthCheck(t *testing.T) {
	exe := tempExecutable(t)
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{Method: scanner.OpenCodeMethodCurl, BinPath: exe})
	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent}
	batch, err := PrepareUpdateBatch(context.Background(), model.CatAgent, []*model.Item{item})
	if err != nil {
		t.Fatal(err)
	}
	prevRun, prevRunner, prevLook := runUpdateCmd, outputRunner, lookPath
	runUpdateCmd = func(context.Context, Options, string, ...string) (string, string, error) {
		return "upgrade done", "", nil
	}
	outputRunner = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("1.18.16"), nil
	}
	lookPath = func(string) (string, error) { return exe, nil }
	t.Cleanup(func() { runUpdateCmd, outputRunner, lookPath = prevRun, prevRunner, prevLook })

	results := ExecutePreparedBatch(context.Background(), batch, SilentOptions())
	if len(results) != 1 || !results[0].Success || item.Status != model.StatusDone {
		t.Fatalf("prepared OpenCode update lost health validation: results=%+v status=%v", results, item.Status)
	}
	if !strings.Contains(results[0].Output, "opencode healthy") {
		t.Fatalf("health validation output missing: %q", results[0].Output)
	}
}
