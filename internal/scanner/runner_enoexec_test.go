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
	return writeProbeScript(t, dir, name, false,
		"printf 'ran:%s\\n' \"$(dirname \"$0\")\"\n")
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

const (
	probeStdoutMark = "stdout-mark"
	probeStderrMark = "stderr-mark"
	probeEnvKey     = "UPDASH_ENOEXEC_MARK"
	probeEnvValue   = "from-child-env"
)

func writeProbeScript(t *testing.T, dir, name string, shebang bool, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "# shebang-less on purpose (the kernel answers ENOEXEC)\n" + body
	if shebang {
		content = "#!/bin/sh\n" + body
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stdoutStderrBody() string {
	return "printf '%s\\n' '" + probeStdoutMark + "'\n" +
		"printf '%s\\n' '" + probeStderrMark + "' >&2\n"
}

func requireUnixSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sh fallback is unix-only")
	}
	if _, err := exec.LookPath(binSh); err != nil {
		t.Skip("no sh on PATH")
	}
}

func TestProbeOutputSemantics(t *testing.T) {
	requireUnixSh(t)
	dir := t.TempDir()

	type runner func(path string) ([]byte, error)
	cases := []struct {
		name       string
		shebang    bool
		run        runner
		wantStderr bool
	}{
		{"execCommand shebang", true, func(p string) ([]byte, error) {
			return execCommand(context.Background(), p)
		}, false},
		{"execCombined shebang", true, func(p string) ([]byte, error) {
			return execCombined(context.Background(), p)
		}, true},
		{"execCommandEnv shebang", true, func(p string) ([]byte, error) {
			return execCommandEnv(context.Background(), nil, p)
		}, false},
		{"execCommand ENOEXEC", false, func(p string) ([]byte, error) {
			return execCommand(context.Background(), p)
		}, false},
		{"execCombined ENOEXEC", false, func(p string) ([]byte, error) {
			return execCombined(context.Background(), p)
		}, true},
		{"execCommandEnv ENOEXEC", false, func(p string) ([]byte, error) {
			return execCommandEnv(context.Background(), nil, p)
		}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProbeScript(t, dir, strings.ReplaceAll(tc.name, " ", "-"), tc.shebang, stdoutStderrBody())
			out, err := tc.run(path)
			if err != nil {
				t.Fatalf("probe failed: %v", err)
			}
			got := string(out)
			if !strings.Contains(got, probeStdoutMark) {
				t.Fatalf("missing stdout mark in %q", got)
			}
			hasErr := strings.Contains(got, probeStderrMark)
			if hasErr != tc.wantStderr {
				t.Fatalf("stderr merged=%v, want %v (output %q)", hasErr, tc.wantStderr, got)
			}
		})
	}
}

func TestExecCommandEnvForwardsEnvOnENOEXECRetry(t *testing.T) {
	requireUnixSh(t)
	dir := t.TempDir()
	path := writeProbeScript(t, dir, "envtool", false, "printf '%s\\n' \"$"+probeEnvKey+"\"\n")
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		probeEnvKey + "=" + probeEnvValue,
	}
	out, err := execCommandEnv(context.Background(), env, path)
	if err != nil {
		t.Fatalf("ENOEXEC env retry failed: %v", err)
	}
	if !strings.Contains(string(out), probeEnvValue) {
		t.Fatalf("child env not forwarded on retry: %q", string(out))
	}
}
