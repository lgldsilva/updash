package cleaner

// ephemeral_reap_test.go — updash#107: reap de containers efêmeros que
// vazaram em Up (docker container prune só remove exited/dead). Opt-in por
// glob de nome + TTL mínimo de uptime.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

func TestDockerEphemeral_DisabledByDefault(t *testing.T) {
	item := &model.Item{Name: "docker ephemeral-reap", Category: model.CatDockerClean}
	r := cleanDockerEphemeral(context.Background(), item, SilentOptions())
	if !r.Success || item.Status != model.StatusInfo {
		t.Fatalf("sem patterns deve ser info no-op: %+v", r)
	}
	if !strings.Contains(r.Output, "disabled") {
		t.Fatalf("output deveria avisar que o reap está desligado: %q", r.Output)
	}
}

func TestDockerEphemeral_ReapsOnlyOldMatchingRunning(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_PATTERNS", "sweep-*,*-e2e-*,*-validation")
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_MAX_HOURS", "12")
	t.Setenv(fakeStdoutEnv, "")
	log := recordExec(t)

	item := &model.Item{Name: "docker ephemeral-reap", Category: model.CatDockerClean}
	r := cleanDockerEphemeral(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %+v (out=%q)", r, r.Output)
	}
	// O fake executa docker ps e devolve stdout vazio — sem containers.
	// O valor do teste é o roteamento: ps --filter status=running foi chamado.
	if got := log.joined(); !strings.Contains(got, "ps --filter status=running") {
		t.Fatalf("deveria listar containers running: %q", got)
	}
}

func TestDockerEphemeral_StopThenRemoveSequence(t *testing.T) {
	// Sequência: para cada vítima, stop -t 10 → rm. Fake stdout devolve uma
	// linha ps com um container antigo que casa o padrão.
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_PATTERNS", "sweep-*")
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_MAX_HOURS", "12")
	t.Setenv(fakeStdoutEnv, "abc123\tsweep-corp-platform-9\t2 days\n")
	log := recordExec(t)

	item := &model.Item{Name: "docker ephemeral-reap", Category: model.CatDockerClean}
	r := cleanDockerEphemeral(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %+v (out=%q)", r, r.Output)
	}
	want := "docker ps --filter status=running --format {{.ID}}\t{{.Names}}\t{{.RunningFor}}\n" +
		"docker stop -t 10 abc123\ndocker rm abc123\n"
	if got := log.joined(); got != want {
		t.Fatalf("sequência errada:\ngot:  %q\nwant: %q", got, want)
	}
	if !strings.Contains(r.Output, "reaped sweep-corp-platform-9") {
		t.Fatalf("output deveria reportar a vítima: %q", r.Output)
	}
}

func TestDockerEphemeral_SparesFreshAndNonMatching(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_PATTERNS", "sweep-*")
	t.Setenv("UPDASH_DOCKER_EPHEMERAL_MAX_HOURS", "12")
	// fresh: casa o padrão mas tem 3 hours; prod: nginx-proxy-manager, não casa.
	t.Setenv(fakeStdoutEnv, "fresh01\tsweep-fresh-1\t3 hours\nprod001\tnpm-manager\t2 months\n")
	log := recordExec(t)

	item := &model.Item{Name: "docker ephemeral-reap", Category: model.CatDockerClean}
	r := cleanDockerEphemeral(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	if got := log.joined(); strings.Contains(got, "stop") || strings.Contains(got, "rm ") {
		t.Fatalf("nenhum container deveria ser reaped (fresh/prod): %q", got)
	}
}

func TestParseDockerRunningFor(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"3 hours", 3 * time.Hour, true},
		{"About a minute", time.Minute, true},
		{"2 days", 48 * time.Hour, true},
		{"14 days", 336 * time.Hour, true},
		{"5 weeks", 365 * 24 * time.Hour, true},
		{"About a second", time.Second, true},
		{"Less than a second", 0, false}, // "Less" não é valor — poupado
		{"garbage", 0, false},
	}
	for _, tc := range cases {
		got, err := parseDockerRunningFor(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("%q: got %v/%v, want %v", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%q: deveria falhar o parse", tc.in)
		}
	}
}

func TestMatchesAnyPattern(t *testing.T) {
	patterns := []string{"sweep-*,*-e2e-*,*-validation"}
	split := strings.Split(patterns[0], ",")
	cases := []struct {
		name string
		want bool
	}{
		{"sweep-corp-platform-9", true},
		{"costs-12-validation", true},
		{"GITEA-ACTIONS-TASK-abc-e2e-1", true},
		{"npm-manager", false},
		{"vpn-gateway", false},
	}
	for _, tc := range cases {
		if got := matchesAnyPattern(tc.name, split); got != tc.want {
			t.Fatalf("%q: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
