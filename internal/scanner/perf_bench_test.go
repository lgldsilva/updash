package scanner

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

// Benchmarks measuring wall-clock of the scan hot paths that the
// perf/scan-optimizations branch targets. Each benchmark installs a mock
// execCommand with realistic per-invocation latency so results are stable
// and independent of the host's installed tooling.

const benchProbeLatency = 50 * time.Millisecond

func installBenchExec(t testing.TB, calls *atomic.Int64) {
	t.Helper()
	oldExec := execCommand
	oldCombined := execCombined
	run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if calls != nil {
			calls.Add(1)
		}
		time.Sleep(benchProbeLatency)
		if len(args) > 0 && args[0] == "view" {
			return []byte("1.2.3\n"), nil
		}
		if name == binNpm && len(args) > 0 && args[0] == "ls" {
			return []byte(`{"dependencies":{"@anthropic-ai/claude-code":{"version":"1.0.0"}}}`), nil
		}
		if name == binNpm && len(args) > 0 && args[0] == "outdated" {
			return []byte(`{}`), nil
		}
		return []byte("1.0.0\n"), nil
	}
	execCommand = run
	execCombined = run
	t.Cleanup(func() { execCommand = oldExec; execCombined = oldCombined })
}

// benchAgentsWithBinary returns n catalog defs whose binaries exist (mocked
// via a temp dir is unnecessary: LookPath uses PATH; we instead build items
// directly and exercise resolveRegistryLatest, skipping LookPath).
func benchAgentItems(n int) ([]*model.Item, []agentDef) {
	items := make([]*model.Item, 0, n)
	catalog := make([]agentDef, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("agent-%02d", i)
		pkg := fmt.Sprintf("@scope/pkg-%02d", i)
		catalog = append(catalog, agentDef{name: name, binary: name, npmPackage: pkg})
		items = append(items, &model.Item{Name: name, Category: model.CatAgent, CurrentVer: "1.0.0"})
	}
	return items, catalog
}

// BenchmarkResolveRegistryLatest measures the post-version-probe phase where
// agents not managed by global npm get their latest version from the registry.
// Baseline (serial): ~n sequential probes. Target: bounded parallel probes.
func BenchmarkResolveRegistryLatest(b *testing.B) {
	ctx := context.Background()
	for _, n := range []int{4, 8} {
		b.Run(fmt.Sprintf("agents-%d", n), func(b *testing.B) {
			var calls atomic.Int64
			installBenchExec(b, &calls)
			items, catalog := benchAgentItems(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fresh := make([]*model.Item, len(items))
				for j, it := range items {
					cp := *it
					fresh[j] = &cp
				}
				resolveRegistryLatest(ctx, fresh, catalog)
			}
		})
	}
}

// BenchmarkEnabledSources guards against duplicate source registration
// (regression test for the double PacmanSource entry).
func TestEnabledSourcesNoDuplicates(t *testing.T) {
	plat := model.PlatformInfo{
		HasPacman: true,
		HasBrew:   true,
		HasNpm:    true,
	}
	srcs := enabledSources(plat, false)
	seen := make(map[string]int)
	for _, s := range srcs {
		seen[fmt.Sprintf("%T", s)]++
	}
	for typ, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate source registered %dx: %s", n, typ)
		}
	}
}
