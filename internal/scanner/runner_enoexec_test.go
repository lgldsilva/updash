package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeShebangLessTool writes an executable, shebang-less sh script like
// pnpm's pre-native placeholder: the kernel refuses it with ENOEXEC and only
// a shell can run it.
func writeShebangLessTool(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "# shebang-less on purpose (the kernel answers ENOEXEC)\n" +
		"printf 'ran:%s\\n' \"$(dirname \"$0\")\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSpawnRetriesShebangLessScriptUnderSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh fallback is unix-only")
	}
	if _, err := exec.LookPath(binSh); err != nil {
		t.Skip("no sh on PATH")
	}
	dir := t.TempDir()
	path := writeShebangLessTool(t, dir, "tool")

	// Absolute path: execve fails with ENOEXEC, sh then interprets the script.
	// $0 must be the real path so the script can locate itself (pnpm's
	// placeholder walks symlinks from $0).
	out, err := execCommand(context.Background(), path, "extra-arg")
	if err != nil {
		t.Fatalf("ENOEXEC retry failed: %v", err)
	}
	if !strings.Contains(string(out), "ran:"+dir) {
		t.Fatalf("bad output from retried script: %q", string(out))
	}
}

func TestSpawnRetriesBareNameViaPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh fallback is unix-only")
	}
	if _, err := exec.LookPath(binSh); err != nil {
		t.Skip("no sh on PATH")
	}
	dir := t.TempDir()
	writeShebangLessTool(t, dir, "uptool")

	// Bare name: the retry must resolve it through PATH before handing it to
	// sh (prepend dir, keep the rest so sh itself stays reachable).
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := execCommand(context.Background(), "uptool")
	if err != nil {
		t.Fatalf("ENOEXEC retry failed: %v", err)
	}
	if !strings.Contains(string(out), "ran:"+dir) {
		t.Fatalf("bad output: %q", string(out))
	}
}

func TestSpawnDoesNotRetryOtherErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh fallback is unix-only")
	}
	// Missing binary is exec.Error, not ENOEXEC: no retry, error as-is.
	_, err := execCommand(context.Background(), "definitely-not-a-real-binary-xyz")
	if err == nil {
		t.Fatal("expected lookup error, got nil")
	}
	if strings.Contains(err.Error(), binSh) {
		t.Fatalf("unexpected sh involvement in error: %v", err)
	}
}
