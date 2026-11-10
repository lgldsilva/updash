package retention

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "extensions.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVSCodeActiveExtensionDirs(t *testing.T) {
	t.Run("relativeLocation and location.path", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, `[
			{"identifier":{"id":"ms-python.python"},"version":"2026.3.0","relativeLocation":"ms-python.python-2026.3.0-universal"},
			{"identifier":{"id":"a.b"},"version":"1.0.0","location":{"$mid":1,"path":"/x/ext/a.b-1.0.0","scheme":"file"}}
		]`)
		got, ok, err := VSCodeActiveExtensionDirs(dir)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		want := map[string]bool{"ms-python.python-2026.3.0-universal": true, "a.b-1.0.0": true}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		got, ok, err := VSCodeActiveExtensionDirs(t.TempDir())
		if err != nil || ok || got != nil {
			t.Fatalf("got=%v ok=%v err=%v", got, ok, err)
		}
	})
	t.Run("corrupt manifest fails closed", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, `{not json`)
		if _, _, err := VSCodeActiveExtensionDirs(dir); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestKeepExtensionDirs(t *testing.T) {
	newestFirst := []string{"x-3.0.0", "x-2.0.0", "x-1.0.0"}
	cases := []struct {
		name   string
		active map[string]bool
		want   []string
	}{
		{"no manifest keeps newest", nil, []string{"x-3.0.0"}},
		{"downgraded keeps active only", map[string]bool{"x-2.0.0": true}, []string{"x-2.0.0"}},
		{"manifest without this ext keeps newest", map[string]bool{"y-1.0.0": true}, []string{"x-3.0.0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := KeepExtensionDirs(newestFirst, tc.active); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if got := KeepExtensionDirs(nil, nil); got != nil {
		t.Fatalf("empty input: got %v", got)
	}
}
