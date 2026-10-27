package scan

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"
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

// QuietEnv is added to every tool nodekill runs.
var QuietEnv = []string{
	"CI=1", "NO_COLOR=1", "FORCE_COLOR=0", "NO_UPDATE_NOTIFIER=1", "npm_config_update_notifier=false",
	"COREPACK_ENABLE_DOWNLOAD_PROMPT=0", "COREPACK_ENABLE_AUTO_PIN=0", "YARN_ENABLE_TELEMETRY=0",
	"NEXT_TELEMETRY_DISABLED=1", "DO_NOT_TRACK=1", "PNPM_DISABLE_SELF_UPDATE_CHECK=true",
}
