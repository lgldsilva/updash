package updater

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// ShellUpdateCommand returns a shell-safe update command for an item.
func ShellUpdateCommand(item *model.Item) string {
	if item == nil {
		return "false"
	}
	switch item.Category {
	case model.CatBrew:
		return "brew upgrade --greedy " + shellQuote(item.Name)
	case model.CatMAS:
		if item.PackageID != "" {
			return "mas update " + shellQuote(item.PackageID)
		}
		return "false"
	case model.CatApt:
		return "sudo apt-get update && sudo apt-get upgrade -y"
	case model.CatSnap:
		return "sudo snap refresh --color=never"
	default:
		return "false"
	}
}

func shellQuote(s string) string {
	return strconv.Quote(s)
}

// DefaultPrivilegedPath is used when PATH is empty inside the macOS authorization context.
const DefaultPrivilegedPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

// macOSNativePrivilegedPreamble runs brew/mas as the console user when the AppleScript
// authorization dialog executed this script as root (Homebrew refuses to run as root).
const macOSNativePrivilegedPreamble = `
set +e
exec 2>&1
REAL_USER="$(stat -f '%Su' /dev/console 2>/dev/null)"
if [ -z "$REAL_USER" ] || [ "$REAL_USER" = "root" ]; then
  REAL_USER="${SUDO_USER:-$USER}"
fi
run_as_user() {
  if [ "$(id -u)" -eq 0 ] && [ -n "$REAL_USER" ] && [ "$REAL_USER" != "root" ]; then
    sudo -u "$REAL_USER" -H /bin/bash -lc "$1"
  else
    /bin/bash -lc "$1"
  fi
}
if [ "$(id -u)" -eq 0 ] && [ -n "$REAL_USER" ] && [ "$REAL_USER" != "root" ]; then
  sudo -u "$REAL_USER" sudo -v 2>/dev/null || true
fi
`

// BuildElevatedShellScript builds a bash script that updates items with UPDASH_OK/FAIL markers.
func BuildElevatedShellScript(items []*model.Item, pathEnv string) string {
	if strings.TrimSpace(pathEnv) == "" {
		pathEnv = DefaultPrivilegedPath
	}
	var b strings.Builder
	b.WriteString(macOSNativePrivilegedPreamble)
	fmt.Fprintf(&b, "export PATH=%s\n", shellQuote(pathEnv))
	for _, it := range items {
		inner := ShellUpdateCommand(it)
		name := shellQuote(it.Name)
		fmt.Fprintf(&b, "echo '→ %s'\n", it.Name)
		fmt.Fprintf(&b, "if run_as_user %s; then printf 'UPDASH_OK %%s\\n' %s; else printf 'UPDASH_FAIL %%s\\n' %s; fi\n",
			shellQuote(inner), name, name)
	}
	return b.String()
}
