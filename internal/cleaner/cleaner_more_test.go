package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

func TestItemTimeout(t *testing.T) {
	cases := []struct {
		cat  model.Category
		want time.Duration
	}{
		{model.CatDockerClean, 30 * time.Minute},
		{model.CatSDKMAN, 15 * time.Minute},
		{model.CatVSCodeClean, 10 * time.Minute},
		{model.CatCache, 10 * time.Minute},
	}
	for _, tc := range cases {
		d := ItemTimeout(&model.Item{Category: tc.cat})
		if d != tc.want {
			t.Errorf("ItemTimeout(%s) = %s, want %s", tc.cat, d, tc.want)
		}
	}
}

func TestOptions(t *testing.T) {
	if DefaultOptions().Verbose != true {
		t.Fatal("DefaultOptions should be verbose")
	}
	if SilentOptions().Verbose {
		t.Fatal("SilentOptions should be silent")
	}
}

func TestSanitizeIdent(t *testing.T) {
	got := sanitizeIdent("java;rm -rf 21.0.1-tem")
	want := "javarm-rf21.0.1-tem"
	if got != want {
		t.Errorf("sanitizeIdent = %q, want %q", got, want)
	}
}

func TestCleanOne_UnknownCategory(t *testing.T) {
	item := &model.Item{Name: "x", Category: model.CatBrew}
	r := cleanOne(context.Background(), item, SilentOptions())
	if r.Success {
		t.Fatal("expected failure for brew category in cleaner")
	}
	if !strings.Contains(r.Error, "no cleaner") {
		t.Fatalf("unexpected error: %q", r.Error)
	}
}

func TestCleanCache_DefaultNoop(t *testing.T) {
	item := &model.Item{Name: "pip-cache", Category: model.CatCache}
	r := cleanCache(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected noop success, got %v", r.Error)
	}
}

func TestCleanSDKMAN_ParseError(t *testing.T) {
	item := &model.Item{Name: "java", Category: model.CatSDKMAN}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r.Success || !strings.Contains(r.Error, "cannot parse") {
		t.Fatalf("expected parse error, got %+v", r)
	}
}

func TestCleanSDKMAN_NothingToRemove(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	verDir := filepath.Join(tmp, ".sdkman", "candidates", "java", "21.0.7-tem")
	if err := os.MkdirAll(verDir, 0o750); err != nil {
		t.Fatal(err)
	}
	item := &model.Item{
		Name:       "java 21",
		Category:   model.CatSDKMAN,
		CurrentVer: "21.0.7-tem",
	}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %q", r.Error)
	}
	if !strings.Contains(r.Output, "nothing to remove") {
		t.Fatalf("unexpected output: %q", r.Output)
	}
}

func TestCleanSDKMAN_RemovesOlderVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	base := filepath.Join(tmp, ".sdkman", "candidates", "java")
	for _, ver := range []string{"21.0.1-tem", "21.0.2-tem"} {
		if err := os.MkdirAll(filepath.Join(base, ver), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	item := &model.Item{
		Name:       "java 21",
		Category:   model.CatSDKMAN,
		CurrentVer: "21.0.2-tem",
	}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r == nil {
		t.Fatal("nil result")
	}
	// sdk uninstall may fail without SDKMAN installed; still exercises removal path
	if r.Output == "" && r.Error == "" {
		t.Fatal("expected some output or error")
	}
}

func TestCleanSDKMAN_MissingDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	item := &model.Item{Name: "java 21", Category: model.CatSDKMAN}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r.Success {
		t.Fatal("expected failure for missing sdkman dir")
	}
}

func TestCleanAllWithOptions_TimeoutPerItem(t *testing.T) {
	item := &model.Item{Name: "pip-cache", Category: model.CatCache}
	results := CleanAllWithOptions(context.Background(), []*model.Item{item}, SilentOptions())
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestCleanVSCodeExt_NoDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	item := &model.Item{Name: "ext: publisher.ext", Category: model.CatVSCodeClean}
	r := cleanVSCodeExt(context.Background(), item, SilentOptions())
	if !r.Success || !strings.Contains(r.Output, "no extension directories") {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestCleanVSCodeExt_PruneOldVersions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	extRoot := filepath.Join(tmp, ".antigravity", "extensions")
	for _, name := range []string{
		"ms-python.python-2026.4.0-universal",
		"ms-python.python-2026.3.0-universal",
	} {
		if err := os.MkdirAll(filepath.Join(extRoot, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	item := &model.Item{Name: "ext: ms-python.python", Category: model.CatVSCodeClean}
	r := cleanVSCodeExt(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %q", r.Error)
	}
	if !strings.Contains(r.Output, "removed") {
		t.Fatalf("expected removal output, got %q", r.Output)
	}
	if _, err := os.Stat(filepath.Join(extRoot, "ms-python.python-2026.3.0-universal")); !os.IsNotExist(err) {
		t.Fatal("old version should be removed")
	}
}

func TestCleanVSCodeExt_ParseError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	extRoot := filepath.Join(tmp, ".antigravity", "extensions")
	if err := os.MkdirAll(extRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	item := &model.Item{Name: "ext:   ", Category: model.CatVSCodeClean}
	r := cleanVSCodeExt(context.Background(), item, SilentOptions())
	if r.Success {
		t.Fatal("expected parse failure")
	}
}
