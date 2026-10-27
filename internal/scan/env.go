package scan

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
)

// Env is how a scan sees the machine. Tests replace it.
type Env struct {
	Home     string
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	// Query runs a read-only command (a tool's "where is your cache" or
	// "list" query) and returns its stdout.
	Query func(dir, name string, args ...string) ([]byte, error)
}

// Snapshot reads the running processes. Tests on systems that cannot see
// process working directories (Windows) replace it.
var Snapshot = procs.Snapshot

// DefaultEnv is the real machine.
func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	return Env{Home: home, GOOS: runtime.GOOS, Getenv: os.Getenv, LookPath: exec.LookPath, Query: query}
}

// query runs a tool with a timeout, no stdin, and settings that keep it
// from prompting, updating itself or phoning home.
func query(dir, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), QuietEnv...)
	return cmd.Output()
}

// QuietEnv is added to every tool nodekill runs, for queries and cleanups
// alike: no prompts, colour, telemetry, update checks or downloads. Corepack
// shims fail instead of fetching a package manager, pnpm does not switch to
// a version pinned in a packageManager field (pmOnFail; managePackageManager-
// Versions before pnpm 11), and `brew uninstall` removes only the formula
// named, not dependencies it would otherwise autoremove.
var QuietEnv = []string{
	"CI=1", "NO_COLOR=1", "FORCE_COLOR=0", "NO_UPDATE_NOTIFIER=1", "DO_NOT_TRACK=1",
	"npm_config_update_notifier=false",
	"pnpm_config_update_notifier=false", "pnpm_config_pm_on_fail=ignore", "npm_config_manage_package_manager_versions=false",
	"COREPACK_ENABLE_DOWNLOAD_PROMPT=0", "COREPACK_ENABLE_AUTO_PIN=0", "COREPACK_ENABLE_NETWORK=0", "COREPACK_DEFAULT_TO_LATEST=0",
	"YARN_ENABLE_TELEMETRY=0", "NEXT_TELEMETRY_DISABLED=1",
	"HOMEBREW_NO_AUTOREMOVE=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_ENV_HINTS=1",
}
