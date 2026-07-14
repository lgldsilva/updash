package cleaner

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestCleanAll_Empty(t *testing.T) {
	results := CleanAll(context.Background(), nil)
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestRunCmd_Success(t *testing.T) {
	item := &model.Item{Name: "noop", Category: model.CatCache}
	r := runCmd(context.Background(), item, SilentOptions(), "true")
	if !r.Success {
		t.Fatalf("expected success, got %q", r.Error)
	}
}

func TestRunCmd_Verbose(t *testing.T) {
	item := &model.Item{Name: "noop", Category: model.CatCache}
	r := runCmd(context.Background(), item, DefaultOptions(), "true")
	if !r.Success {
		t.Fatalf("verbose run failed: %q", r.Error)
	}
}

func TestRunMultiElevatedCmd(t *testing.T) {
	item := &model.Item{Name: "apt-cache", Category: model.CatCache}
	r := runMultiElevatedCmd(context.Background(), item, SilentOptions(), []string{"true"})
	if r == nil {
		t.Fatal("nil result")
	}
}

func TestCleanCache_Apt(t *testing.T) {
	item := &model.Item{Name: "apt-cache", Category: model.CatCache}
	r := cleanCache(context.Background(), item, SilentOptions())
	if r == nil {
		t.Fatal("nil result")
	}
}

func TestCleanOne_Categories(t *testing.T) {
	ctx := context.Background()
	opts := SilentOptions()
	cases := []struct {
		cat  model.Category
		name string
	}{
		{model.CatCache, "misc-cache"},
		{model.CatSDKMAN, "java 21"},
		{model.CatDockerClean, "docker images"},
		{model.CatVSCodeClean, "ext: foo"},
	}
	for _, tc := range cases {
		item := &model.Item{Name: tc.name, Category: tc.cat}
		r := cleanOne(ctx, item, opts)
		if r == nil {
			t.Fatalf("nil for %s", tc.cat)
		}
	}
}

func TestRunCmd_Failure(t *testing.T) {
	item := &model.Item{Name: "fail", Category: model.CatCache}
	r := runCmd(context.Background(), item, SilentOptions(), "false")
	if r.Success {
		t.Fatal("expected failure")
	}
}

func TestCleanDocker_Routes(t *testing.T) {
	ctx := context.Background()
	opts := SilentOptions()
	names := []string{"docker images", "docker build cache", "docker containers", "docker volumes", "docker misc"}
	for _, name := range names {
		item := &model.Item{Name: name, Category: model.CatDockerClean}
		r := cleanDocker(ctx, item, opts)
		if r == nil {
			t.Fatalf("nil result for %s", name)
		}
	}
}

func TestCleanWindowsCache_Default(t *testing.T) {
	item := &model.Item{Name: "win misc", Category: model.CatCache}
	r := cleanWindowsCache(context.Background(), item, SilentOptions())
	if r == nil {
		t.Fatal("expected result")
	}
}
