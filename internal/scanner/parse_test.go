package scanner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// TestBrewJSONParsing tests that the brew JSON structs unmarshal correctly.
func TestBrewJSONParsing(t *testing.T) {
	sampleJSON := `{
		"formulae": [
			{"name": "btop", "installed_versions": ["1.3.0"], "current_version": "1.5.0", "pinned": false}
		],
		"casks": [
			{"name": "vlc", "installed_versions": ["3.0.18"], "current_version": "3.0.23", "pinned": false}
		]
	}`

	var data brewOutdatedJSON
	if err := json.Unmarshal([]byte(sampleJSON), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(data.Formulae) != 1 {
		t.Errorf("expected 1 formula, got %d", len(data.Formulae))
	}
	if len(data.Casks) != 1 {
		t.Errorf("expected 1 cask, got %d", len(data.Casks))
	}
	if data.Formulae[0].Name != "btop" {
		t.Errorf("formula name = %q, want %q", data.Formulae[0].Name, "btop")
	}
	if data.Casks[0].Name != "vlc" {
		t.Errorf("cask name = %q, want %q", data.Casks[0].Name, "vlc")
	}
}

func TestWingetJSONParsing(t *testing.T) {
	sampleJSON := `{"upgrades": [
		{"name": "7zip", "id": "7zip.7zip", "version": "23.01", "newVersion": "24.01"},
		{"name": "Git", "id": "Git.Git", "version": "2.42.0", "newVersion": "2.45.0"}
	]}`

	var data wingetUpgradeJSON
	if err := json.Unmarshal([]byte(sampleJSON), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(data.Upgrades) != 2 {
		t.Errorf("expected 2 upgrades, got %d", len(data.Upgrades))
	}
	if data.Upgrades[0].Name != "7zip" {
		t.Errorf("upgrade[0].Name = %q, want %q", data.Upgrades[0].Name, "7zip")
	}
}

// TestChocoOutputParsing tests parsing of choco outdated output.
func TestChocoOutputParsing(t *testing.T) {
	sampleOutput := `chocolatey|1.2.0|2.0.0|false
git|2.42.0|2.45.0|false
nodejs|18.0.0|20.0.0|false`

	lines := strings.Split(strings.TrimSpace(sampleOutput), "\n")
	var items []*model.Item

	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			items = append(items, &model.Item{
				Name:         strings.TrimSpace(parts[0]),
				CurrentVer:   strings.TrimSpace(parts[1]),
				AvailableVer: strings.TrimSpace(parts[2]),
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Name != "chocolatey" {
		t.Errorf("item[0].Name = %q, want %q", items[0].Name, "chocolatey")
	}
}

// TestMASOutputParsing tests the text parsing from mas outdated.
func TestMASOutputParsing(t *testing.T) {
	sampleOutput := `1234567890  Xcode (16.0 -> 16.1)
497799835  Pixelmator Pro (3.5.0 -> 3.6.0)`

	lines := strings.Split(strings.TrimSpace(sampleOutput), "\n")
	var items []*model.Item

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := strings.Join(parts[1:], " ")
		cur := ""
		avail := ""

		if idx := strings.Index(name, "("); idx >= 0 {
			verPart := name[idx:]
			name = strings.TrimSpace(name[:idx])
			verPart = strings.TrimPrefix(verPart, "(")
			verPart = strings.TrimSuffix(verPart, ")")
			if arrow := strings.Index(verPart, "->"); arrow >= 0 {
				cur = strings.TrimSpace(verPart[:arrow])
				avail = strings.TrimSpace(verPart[arrow+2:])
			}
		}

		items = append(items, &model.Item{
			Name:         name,
			CurrentVer:   cur,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "Xcode" {
		t.Errorf("item[0].Name = %q, want %q", items[0].Name, "Xcode")
	}
	if items[0].AvailableVer != "16.1" {
		t.Errorf("item[0].AvailableVer = %q, want %q", items[0].AvailableVer, "16.1")
	}
}

// TestScoopOutputParsing tests parsing of scoop status output.
func TestScoopOutputParsing(t *testing.T) {
	sampleOutput := `WARN  Scoop is out of date.
Updates are available for the following packages:
    aria2: 1.36.0-1 (latest: 1.37.0-1)
    git: 2.42.0.windows.1 (latest: 2.45.0.windows.1)
Everything is ok!`

	items := parseScoopStatus(sampleOutput)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "aria2" {
		t.Errorf("item[0].Name = %q, want %q", items[0].Name, "aria2")
	}
	if items[1].Name != "git" {
		t.Errorf("item[1].Name = %q, want %q", items[1].Name, "git")
	}
}

// TestAptParsing tests parsing of apt list --upgradable.
func TestAptParsing(t *testing.T) {
	sampleOutput := `Listing...
libssl3/stable 3.1.0 amd64 [upgradable from: 3.0.13]
git/stable 2.45.0 amd64 [upgradable from: 2.43.0]`

	lines := strings.Split(strings.TrimSpace(sampleOutput), "\n")
	var items []*model.Item

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[:idx]
		}
		avail := parts[1]
		cur := ""
		if idx := strings.Index(line, "from:"); idx >= 0 {
			rest := strings.TrimSpace(line[idx+5:])
			rest = strings.TrimSuffix(rest, "]")
			cur = rest
		}
		items = append(items, &model.Item{
			Name: name, CurrentVer: cur, AvailableVer: avail, Status: model.StatusOutdated,
		})
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "libssl3" {
		t.Errorf("item[0].Name = %q, want %q", items[0].Name, "libssl3")
	}
	if items[1].CurrentVer != "2.43.0" {
		t.Errorf("item[1].CurrentVer = %q, want %q", items[1].CurrentVer, "2.43.0")
	}
}

// TestSnapParsing tests parsing of snap refresh --list output.
func TestSnapParsing(t *testing.T) {
	sampleOutput := `Name    Version   Rev   Tracking  Publisher  Notes
core22  2024-01   1234  latest/stable  canonical✓  -
firefox 123.0    5678  latest/stable  mozilla✓    -`

	lines := strings.Split(strings.TrimSpace(sampleOutput), "\n")
	var items []*model.Item
	header := true

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || header {
			header = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			items = append(items, &model.Item{
				Name: fields[0], AvailableVer: fields[1], Status: model.StatusOutdated,
			})
		}
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "core22" {
		t.Errorf("item[0].Name = %q, want %q", items[0].Name, "core22")
	}
}

// TestNpmJSONParsing tests parsing of npm outdated --json.
func TestNpmJSONParsing(t *testing.T) {
	sampleJSON := `{
		"@charmland/crush": {"current": "0.79.1", "wanted": "0.84.1", "latest": "0.84.1"},
		"npm": {"current": "11.17.0", "wanted": "12.0.1", "latest": "12.0.1"}
	}`

	var data map[string]struct {
		Current string `json:"current"`
		Wanted  string `json:"wanted"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal([]byte(sampleJSON), &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(data) != 2 {
		t.Errorf("expected 2 packages, got %d", len(data))
	}
	if data["npm"].Latest != "12.0.1" {
		t.Errorf("npm latest = %q, want %q", data["npm"].Latest, "12.0.1")
	}
}

// TestEnabledSources_MacOS_Full verifies all macOS sources are enabled.
func TestEnabledSources_MacOS_Full(t *testing.T) {
	plat := model.PlatformInfo{
		OS: "darwin", Distro: "macos",
		HasBrew: true, HasMAS: true,
		HasNpm: true, HasPipx: true, HasGo: true, HasGup: true,
		HasRustup: true, HasCargo: true, HasSDKMAN: true,
		HasDocker: true, HasNvm: true, HasOmz: true,
	}

	srcs := enabledSources(plat, false)

	cats := make(map[model.Category]bool)
	for _, s := range srcs {
		cats[s.Category()] = true
	}

	expected := []model.Category{
		model.CatBrew, model.CatMAS, model.CatNpm, model.CatPipx,
		model.CatGo, model.CatRustup, model.CatCargo, model.CatDocker,
		model.CatNvm, model.CatOmz, model.CatAgent, model.CatAI,
	}

	for _, c := range expected {
		if !cats[c] {
			t.Errorf("expected category %s to be enabled on macOS", c)
		}
	}
}

// TestEnabledSources_Windows verifies Windows sources are enabled.
func TestEnabledSources_Windows(t *testing.T) {
	plat := model.PlatformInfo{
		OS: "windows", Distro: "windows",
		HasWinget: true, HasChoco: true, HasScoop: true,
		HasNpm: true, HasGo: true, HasDocker: true,
	}

	srcs := enabledSources(plat, false)

	cats := make(map[model.Category]bool)
	for _, s := range srcs {
		cats[s.Category()] = true
	}

	expected := []model.Category{
		model.CatWinget, model.CatChoco, model.CatScoop,
		model.CatNpm, model.CatGo, model.CatDocker,
	}

	for _, c := range expected {
		if !cats[c] {
			t.Errorf("expected category %s to be enabled on Windows", c)
		}
	}
}

// TestEnabledSources_Linux verifies Linux sources are enabled.
func TestEnabledSources_Linux(t *testing.T) {
	plat := model.PlatformInfo{
		OS: "linux", Distro: "ubuntu",
		HasApt: true, HasSnap: true, HasFlatpak: true,
		HasNpm: true, HasDocker: true, HasSDKMAN: true,
	}

	srcs := enabledSources(plat, false)

	cats := make(map[model.Category]bool)
	for _, s := range srcs {
		cats[s.Category()] = true
	}

	expected := []model.Category{
		model.CatApt, model.CatSnap, model.CatFlatpak,
		model.CatNpm, model.CatDocker, model.CatAgent, model.CatAI,
	}

	for _, c := range expected {
		if !cats[c] {
			t.Errorf("expected category %s to be enabled on Linux", c)
		}
	}
}

// TestEnabledSources_NoTools_Empty returns no sources for empty platform.
func TestEnabledSources_NoTools_Empty(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux"}
	srcs := enabledSources(plat, false)
	// Should have only agent + AI infra (probe always)
	for _, s := range srcs {
		if s.Category() == model.CatAgent || s.Category() == model.CatAI {
			continue
		}
		t.Errorf("unexpected source %s for empty platform", s.Category())
	}
}
