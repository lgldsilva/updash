package cleaner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lgldsilva/updash/internal/retention"
)

// Non-root runs can list containers through the docker daemon but can never
// stat or truncate the json-file logs under /var/lib/docker (0700 root). The
// cleaner classifies that wall as os.ErrPermission and reports it as
// information instead of failing the item — pin the classification here.
func TestTruncateOversizedLogPermissionErrorIsClassified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission wall is unix-only")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads through permission bits; nothing to classify")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "big-json.log")
	if err := os.WriteFile(path, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, _, err := retention.TruncateFileIfOver(path, 1024)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("want os.ErrPermission for unreadable oversized log, got %v", err)
	}
}
