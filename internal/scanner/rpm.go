package scanner

import (
	"context"
	"os/exec"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// rpmCheckExitCode is returned by `dnf check-update` / `yum check-update`
// when updates are available (0 = up to date, other non-zero = error).
const rpmCheckExitCode = 100

// RpmSource scans Fedora/RHEL-family packages via dnf (or yum fallback).
type RpmSource struct{}

func (s *RpmSource) Category() model.Category { return model.CatDnf }
func (s *RpmSource) Label() string            { return binDnf }
func (s *RpmSource) Icon() string             { return "🐧" }

// RpmToolName returns the binary used for scans/updates ("dnf" preferred, "yum" fallback).
func RpmToolName() string {
	if _, err := exec.LookPath(binDnf); err == nil {
		return binDnf
	}
	if _, err := exec.LookPath("dnf5"); err == nil {
		return "dnf5"
	}
	return "yum"
}

func (s *RpmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// check-update exits 100 when updates exist; treat that as success.
	out, err := execCombined(ctx, RpmToolName(), "check-update", "-q")
	if err != nil && !isExitCode(err, rpmCheckExitCode) {
		return []*model.Item{errItem(RpmToolName(), model.CatDnf)}, nil
	}
	return okOrOutdated(RpmToolName(), model.CatDnf, ParseDnfCheckUpdate(string(out))), nil
}

// isExitCode reports whether err is an *exec.ExitError carrying the given code.
func isExitCode(err error, code int) bool {
	exitErr, ok := err.(*exec.ExitError)
	return ok && exitErr.ExitCode() == code
}

// rpmArchSuffixes are stripped from "name.arch" check-update entries.
var rpmArchSuffixes = []string{
	".x86_64", ".aarch64", ".noarch", ".i686", ".armv7hl", ".ppc64le", ".s390x", ".src",
}

// ParseDnfCheckUpdate parses `dnf check-update` output lines of the form:
//
//	name.arch                     version-release              repo
//
// Non-package chatter (metadata notices, "Security:" advisory lines) is skipped.
// Installed versions are not reported by check-update, so CurrentVer stays empty.
func ParseDnfCheckUpdate(output string) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, ":") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		items = append(items, &model.Item{
			Name:         rpmStripArch(fields[0]),
			Category:     model.CatDnf,
			AvailableVer: fields[1],
			Status:       model.StatusOutdated,
		})
	}
	return items
}

// rpmStripArch removes a trailing architecture suffix from a package name.
func rpmStripArch(nameArch string) string {
	for _, arch := range rpmArchSuffixes {
		if strings.HasSuffix(nameArch, arch) {
			return strings.TrimSuffix(nameArch, arch)
		}
	}
	return nameArch
}
