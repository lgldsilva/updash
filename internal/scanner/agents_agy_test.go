package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestAgentUpdatesWithoutFreshness_OnlyAgy(t *testing.T) {
	var got []string
	for _, a := range agentCatalog() {
		if AgentUpdatesWithoutFreshness(a.name) {
			got = append(got, a.name)
		}
	}
	if len(got) != 1 || got[0] != "Agy" {
		t.Fatalf("agents without a freshness channel = %v, want [Agy]", got)
	}
	cmd := AgentUpdateCommand("Agy")
	if len(cmd) != 2 || cmd[0] != "agy" || cmd[1] != "update" {
		t.Fatalf("AgentUpdateCommand(Agy) = %v, want [agy update]", cmd)
	}
	if AgentKeepPolicy("Agy") != "" {
		t.Fatal("auto agent must not expose a catalog keep policy")
	}
	note := AgentUpdateCheckNote("Agy")
	if note != "checked on update: agy update" || strings.Contains(strings.ToLower(note), "manual") {
		t.Fatalf("note = %q", note)
	}
	if AgentUpdatesWithoutFreshness("Grok") || AgentUpdatesWithoutFreshness("Claude Code") ||
		AgentUpdatesWithoutFreshness("Cursor") || AgentUpdatesWithoutFreshness("Antigravity") {
		t.Fatal("agents with a freshness channel or manual mode must not match")
	}
	if AgentUpdateCheckNote("Grok") != "" || AgentUpdateCheckNote("no-such") != "" {
		t.Fatal("only a blind-update agent has a check note")
	}
	if AgentUpdateCommand("Antigravity") != nil || AgentKeepPolicy("Antigravity") == "" {
		t.Fatal("Antigravity must stay manual")
	}
}

func TestClassifyAgyUpdateOutput(t *testing.T) {
	cases := []struct {
		out  string
		want string
	}{
		{"✓ Update successful!\n", AgyUpdateUpdated},
		{"prefix\n✓ Update successful!", AgyUpdateUpdated},
		{"✓ You are already on the latest version.", AgyUpdateCurrent},
		{"already on the latest version", AgyUpdateCurrent},
		{"⟳ Downloading update...", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ClassifyAgyUpdateOutput(tc.out); got != tc.want {
			t.Errorf("ClassifyAgyUpdateOutput(%q) = %q, want %q", tc.out, got, tc.want)
		}
	}
}

func TestAgentScan_AgyVersionIsInfoNotUnverified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("agy fixture is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.2.16; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "agy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	items, err := (&AgentSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	it := items[0]
	if it.Name != "Agy" || it.Status != model.StatusInfo || it.CurrentVer != "1.2.16" {
		t.Fatalf("agy scan = %+v", it)
	}
	if it.KeepPolicy != AgentUpdateCheckNote("Agy") {
		t.Fatalf("keep policy = %q", it.KeepPolicy)
	}
}

func TestAgentScan_AgyVersionFailureStaysUnverified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("agy fixture is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agy"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	items, err := (&AgentSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	it := items[0]
	if it.Status != model.StatusUnverified || it.KeepPolicy != "" {
		t.Fatalf("failed probe must stay unverified without a check note: %+v", it)
	}
}

func TestResolveRegistryLatestFrom_DoesNotProbeAgy(t *testing.T) {
	enableMocks()
	defer disableMocks()

	calls := 0
	prev := execCommand
	execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "agy" {
			calls++
		}
		return prev(ctx, name, args...)
	}
	t.Cleanup(func() { execCommand = prev })

	it := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	resolveRegistryLatestFrom(context.Background(), []*model.Item{it}, agentCatalog(), nil)
	if it.Status != model.StatusInfo || it.AvailableVer != "" {
		t.Fatalf("agy must stay informational when it has no freshness channel: %+v", it)
	}
	if calls != 0 {
		t.Fatalf("agy freshness probe ran %d times", calls)
	}
}
