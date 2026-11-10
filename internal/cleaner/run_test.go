package cleaner

import (
	"context"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

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
	log := recordExec(t)
	item := &model.Item{Name: "apt-cache", Category: model.CatCache}
	r := cleanCache(context.Background(), item, SilentOptions())
	if !r.Success || item.Status != model.StatusCleaned {
		t.Fatalf("expected success, got %+v", r)
	}
	want := "sudo apt-get autoremove -y\nsudo apt-get autoclean\n"
	if got := log.joined(); got != want {
		t.Fatalf("apt cleanup must run elevated: got %q, want %q", got, want)
	}
}

func TestCleanCache_AptFailureMarksError(t *testing.T) {
	recordExec(t)
	t.Setenv(fakeFailEnv, "autoclean")
	item := &model.Item{Name: "apt-cache", Category: model.CatCache}
	r := cleanCache(context.Background(), item, SilentOptions())
	if r.Success || item.Status != model.StatusError {
		t.Fatalf("expected failure, got %+v", r)
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
		log := recordExec(t)
		item := &model.Item{Name: name, Category: model.CatDockerClean}
		r := cleanDocker(ctx, item, opts)
		if !r.Success {
			t.Fatalf("%s: expected success, got %+v", name, r)
		}
		want := "docker " + strings.Join(dockerPruneArgsForItem(name), " ") + "\n"
		if got := log.joined(); got != want {
			t.Fatalf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestDockerPruneArgsForItem(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "all")
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "12h")
	t.Setenv("UPDASH_DOCKER_CONTAINER_MAX_AGE", "6h")

	cases := []struct {
		name string
		want []string
	}{
		{"docker images", []string{"image", "prune", "-a", "--filter", "until=12h", "-f"}},
		{"docker build cache", []string{"builder", "prune", "-af"}},
		{"docker containers", []string{"container", "prune", "-f", "--filter", "until=6h"}},
		{"docker volumes", []string{"volume", "prune", "-f"}},
		{"docker misc", []string{"system", "prune", "-af", "--filter", "until=12h"}},
	}
	for _, tc := range cases {
		got := dockerPruneArgsForItem(tc.name)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
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
