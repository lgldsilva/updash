package updater

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lgldsilva/updash/internal/scanner"
)

// inactivityWindow is how long an update command may produce no output at all
// before it is treated as stuck. Package managers stream progress while they
// work, so a long silence almost always means the child is waiting on an
// interactive prompt it can never receive (updash gives its children no stdin).
// Variable so tests can shrink the window.
var inactivityWindow = 5 * time.Minute

// stallWaitDelay bounds how long Wait may block after the child is killed.
// Output is copied through pipes, and Wait only returns once every holder of
// the write end is gone — including grandchildren the killed process spawned.
// Without a delay a stuck command would still hang the run after the kill.
// Variable so tests can shrink it.
var stallWaitDelay = 5 * time.Second

// nonInteractiveEnvVars are appended to every update command that does not
// already set them. They tell well-behaved tools to skip confirmations and
// colour codes instead of blocking on a TTY that is not there.
// TERM is deliberately left alone: rewriting it breaks brew/pacman output.
var nonInteractiveEnvVars = []string{
	"CI=1",
	"NONINTERACTIVE=1",
	"DEBIAN_FRONTEND=noninteractive",
	"NO_COLOR=1",
}

// prepareUpdateCmd applies the shared execution policy for update commands.
//
// stdin: never wired to the terminal. Go maps a nil Stdin to /dev/null, which
// is what we want; an elevation helper that already piped a password keeps it.
func prepareUpdateCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Env = withNonInteractiveEnv(cmd.Env)
	// pnpm refuses to run global commands when its global bin dir is missing
	// from PATH, whatever the user's shell config says. See EnsurePnpmPath.
	if len(cmd.Args) > 0 && strings.TrimSuffix(filepath.Base(cmd.Args[0]), ".exe") == "pnpm" {
		cmd.Env = scanner.EnsurePnpmPath(cmd.Env)
	}
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = stallWaitDelay
	}
}

// withNonInteractiveEnv returns env plus the non-interactive variables that are
// not already present. A caller-provided value always wins.
func withNonInteractiveEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	present := make(map[string]bool, len(env))
	for _, kv := range env {
		if key, _, ok := strings.Cut(kv, "="); ok {
			present[key] = true
		}
	}
	out := append([]string(nil), env...)
	for _, kv := range nonInteractiveEnvVars {
		key, _, _ := strings.Cut(kv, "=")
		if !present[key] {
			out = append(out, kv)
		}
	}
	return out
}

// activityWriter forwards writes and records when the last byte arrived.
type activityWriter struct {
	dst  io.Writer
	mu   sync.Mutex
	last time.Time
}

func newActivityWriter(dst io.Writer) *activityWriter {
	return &activityWriter{dst: dst, last: time.Now()}
}

func (w *activityWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.last = time.Now()
	w.mu.Unlock()
	if w.dst == nil {
		return len(p), nil
	}
	return w.dst.Write(p)
}

func (w *activityWriter) idle() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return time.Since(w.last)
}

// inactivityError reports a command killed for producing no output.
type inactivityError struct {
	command string
	window  time.Duration
}

func (e *inactivityError) Error() string {
	return fmt.Sprintf("no output for %s — %q may be waiting on an interactive prompt (updash runs updates without stdin); run it manually to answer it",
		e.window, e.command)
}

// runGuarded starts cmd, watches its output for silence and kills it when the
// inactivity window elapses. Behaves exactly like cmd.Run() otherwise.
func runGuarded(cmd *exec.Cmd, window time.Duration) error {
	prepareUpdateCmd(cmd)
	if window <= 0 {
		return cmd.Run()
	}

	watcher := newActivityWriter(nil)
	cmd.Stdout = teeActivity(cmd.Stdout, watcher)
	cmd.Stderr = teeActivity(cmd.Stderr, watcher)

	if err := cmd.Start(); err != nil {
		return err
	}

	var stalled atomic.Bool
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(tickFor(window))
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if watcher.idle() < window {
					continue
				}
				stalled.Store(true)
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				return
			}
		}
	}()

	err := cmd.Wait()
	close(done)
	if stalled.Load() {
		return &inactivityError{command: commandLine(cmd), window: window}
	}
	return err
}

// teeActivity wires a writer that both forwards to dst and marks activity.
func teeActivity(dst io.Writer, watcher *activityWriter) io.Writer {
	return io.MultiWriter(watcher, orDiscard(dst))
}

func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

// tickFor polls often enough to react promptly without busy-looping.
func tickFor(window time.Duration) time.Duration {
	tick := window / 4
	if tick < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return tick
}

func commandLine(cmd *exec.Cmd) string {
	if cmd == nil {
		return ""
	}
	return strings.Join(cmd.Args, " ")
}
