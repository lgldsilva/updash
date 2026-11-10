package retention

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSDKMANDefaultVersion(t *testing.T) {
	t.Run("absolute symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "21.0.3-tem")
		if err := os.Mkdir(target, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "current")); err != nil {
			t.Fatal(err)
		}
		if got, known := SDKMANDefaultVersion(dir); got != "21.0.3-tem" || !known {
			t.Fatalf("got %q, want 21.0.3-tem", got)
		}
	})
	t.Run("relative symlink with trailing slash", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink("17.0.9-tem/", filepath.Join(dir, "current")); err != nil {
			t.Fatal(err)
		}
		if got, known := SDKMANDefaultVersion(dir); got != "17.0.9-tem" || !known {
			t.Fatalf("got %q, want 17.0.9-tem", got)
		}
	})
	t.Run("no current link", func(t *testing.T) {
		if got, known := SDKMANDefaultVersion(t.TempDir()); got != "" || !known {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("current is a copied directory (MSYS) is unknown", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "current"), 0o750); err != nil {
			t.Fatal(err)
		}
		if got, known := SDKMANDefaultVersion(dir); got != "" || known {
			t.Fatalf("got %q known=%v, want unknown", got, known)
		}
	})
}
