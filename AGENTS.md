# nodekill

TUI/CLI that reclaims disk space from the Node.js footprint: project artifacts (`node_modules`, `.next`, framework caches, build output), package-manager caches and stores, npx entries, global packages and Node versions. Go + Bubble Tea v2 + Lip Gloss v2 (+ charmbracelet/x/ansi, golang.org/x/sys). macOS first, Linux second, Windows best effort. Family rules: `../AGENTS.md`.

## Layout
- `internal/scan`: discovery and evidence, read-only. `walk.go` (parallel project walk; never enters dependency/build dirs, tool homes, `.app`, cloud placeholders, privacy folders), `project.go` (package.json, package manager, tools), `rules.go` (the catalog: position + marker + level), `scan.go` (git evidence in one batch per repo, activity, liveness, `classify`, fingerprints, measuring), `globals.go` (caches, npx, global packages), `versions.go` (Node versions), `refresh.go` (re-read one item before acting).
- `internal/size`: parallel du-like sizing; hard links once; APFS private size (`ATTR_CMNEXT_PRIVATESIZE`) as `shared`.
- `internal/clean`: the only code that removes anything. `Act` = refresh, fingerprint check, level check, `guard.go`, then trash/delete or the official command; statfs before/after.
- `internal/contract`: JSON contract (schema_version 1), `docs/contract.md`. `internal/report`: text and receipt. `internal/ui`: Bubble Tea model/view. `internal/testfix`: temp projects and repos for tests.
- `docs/rules.md`: sources for every catalog entry and command. `packaging/homebrew/nodekill.rb`: tap formula template; `RELEASING.md`.

## Invariants
- Project items match only at their catalog position next to the marker that proves the tool (`Rule.When`), never by name. Generic outputs (`dist`, `build`, `out`, `coverage`, `storybook-static`) are review at best and protected outside git. Remotion `out` is protected.
- Inside git: git must ignore the item and track nothing in it (asked with `-C <toplevel>`, top-relative paths); any git error means protected.
- Machine-wide items are removed only by their owning tool's command (fixed argv, absolute executable from an allowlist, never a shell except nvm's documented `nvm.sh`). No command → Trash only for plain download caches; tool missing → shown, protected, with the command. Never remove `npm`/`corepack`, the active Node or a manager's default.
- Every action re-reads the item (`scan.Refresh`), refuses on fingerprint mismatch, re-checks level (review needs explicit selection; `--yes` takes safe project items only) and `clean.Check`. Symlinks are never followed or removed through.
- Default is Trash; `--permanent` deletes folders. `--dry-run`, `--list`, `--json`, `scan` never change anything. Tool queries pass npm's quiet flags (`--logs-max=0 --no-update-notifier`) so even reads leave no logs.
- nodekill writes no config, cache or state and makes no network requests. The contract only grows within a schema version.

## Verify
`gofmt -l .` (empty), `go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=windows go vet ./...`, `go test -race ./...`. Removal is tested only in temp dirs with the trash swapped (`clean.moveFunc`). On real directories use `--list`, `scan --json` or `--dry-run`.
