package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// Builds a Caskroom-style tree and returns the symlink npm would collide with.
func caskroomSymlink(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "Caskroom", "codex", "0.153.4", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(bin, "codex")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestBrewCaskPathFromNpmCollision(t *testing.T) {
	link := caskroomSymlink(t)
	output := "npm error code EEXIST\nnpm error File exists: " + link + "\nnpm error Remove the existing file and try again\n"

	path, ok := brewCaskPathFromNpmCollision(output)
	if !ok {
		t.Fatal("want collision detected for Caskroom symlink")
	}
	if path != link {
		t.Fatalf("path = %q, want %q", path, link)
	}
}

func TestBrewCaskPathFromNpmCollisionNegative(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "tool")
	if err := os.WriteFile(plain, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	otherLink := filepath.Join(dir, "other")
	if err := os.Symlink(plain, otherLink); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"symlink outside Caskroom": "npm error File exists: " + otherLink,
		"regular file":             "npm error File exists: " + plain,
		"missing path":             "npm error File exists: /nonexistent/tool",
		"no marker":                "npm error something else",
	}
	for name, output := range cases {
		if _, ok := brewCaskPathFromNpmCollision(output); ok {
			t.Fatalf("%s: did not expect collision", name)
		}
	}
}

func TestNpmBrewCollisionDowngrade(t *testing.T) {
	link := caskroomSymlink(t)
	item := &model.Item{Name: "Codex", Category: model.CatAgent, PackageID: "@openai/codex"}
	failed := &Result{
		Item:    item,
		Success: false,
		Error:   "exit status 1",
		Output:  "npm error File exists: " + link + "\n",
	}

	got := npmBrewCollisionDowngrade(item, failed)
	if got.Success {
		t.Fatal("downgraded result must not claim success")
	}
	if item.Status != model.StatusOutdated {
		t.Fatalf("status = %v, want StatusOutdated", item.Status)
	}
	if !strings.Contains(item.KeepPolicy, "brew uninstall --cask codex") {
		t.Fatalf("keep policy missing cask removal: %q", item.KeepPolicy)
	}
	if !strings.Contains(item.KeepPolicy, "npm rm -g @openai/codex") {
		t.Fatalf("keep policy missing npm removal: %q", item.KeepPolicy)
	}
	if !strings.HasPrefix(got.Error, "⊘") {
		t.Fatalf("error = %q, want manual-only marker", got.Error)
	}
}

func TestNpmBrewCollisionDowngradePassThrough(t *testing.T) {
	item := &model.Item{Name: "Qwen Code", Category: model.CatAgent, PackageID: "@qwen-code/qwen-code"}

	okResult := &Result{Item: item, Success: true}
	if npmBrewCollisionDowngrade(item, okResult) != okResult {
		t.Fatal("successful result must pass through untouched")
	}

	noCollision := &Result{Item: item, Success: false, Error: "exit status 1", Output: "npm error network unreachable"}
	if npmBrewCollisionDowngrade(item, noCollision) != noCollision {
		t.Fatal("non-collision failure must pass through untouched")
	}
}
