# Contributing

Thanks for helping. nodekill aims to stay small, safe and easy to audit.

## Development

```sh
gofmt -l .
go vet ./...
GOOS=linux go vet ./...
GOOS=windows go vet ./...
go test -race ./...
go run . --dry-run
```

Tests build throwaway projects and git repositories in temp dirs (`internal/testfix`) and swap the trash for a temp folder. Never test removal against real directories; use `--dry-run`, `--list` or `scan --json` there.

## Ground rules

- **The official way only.** A cache, global package or Node version is removed by its own tool's command. When a tool has no command, only plain download caches may move to the Trash. When the tool is missing, show the command and leave the item alone.
- **Read, never write, other tools' state.** Parse configs and lockfiles read-only. When a format is unknown, fail closed: protect the item and say why.
- **Position, not names.** A new project artifact is one `Rule` in `internal/scan/rules.go`: its path below `package.json`, the dependency or config file that proves the tool, its level, what regenerates it, a test in `scan_test.go`, a README row and a source link in `docs/rules.md`.
- All removal goes through `internal/clean`, and every target is re-read and checked right before acting. Keep it that way.
- The JSON contract (`internal/contract`, `docs/contract.md`) only grows within a schema version.
- Keep dependencies to Charm modules and `golang.org/x/sys`.
- No real paths, usernames or project names in code, tests, docs or screenshots. Use placeholders such as `~/dev/my-app`.
- Pull requests are squash-merged, so the PR title becomes the release-notes entry. Use a conventional prefix: `feat:`, `fix:`, `docs:`, `test:`, `ci:` or `chore:` (the last four are left out of release notes).

## Reporting security issues

See [SECURITY.md](SECURITY.md).
