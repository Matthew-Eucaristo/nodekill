package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

const ignoreMore = ignoreAll + "test-results/\nplaywright-report/\nblob-report/\n.netlify/\n.sst/\n.mastra/\n.data/\nplaywright/.auth/\n.turbo/\nsrc-tauri/target/\ncache/\nartifacts/\n"

func TestHoistedToolsMatchWorkspaceMembers(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	mono := filepath.Join(root, "mono")
	testfix.Write(t, mono, map[string]string{
		"package.json":   `{"name":"mono","private":true,"workspaces":["apps/*"],"devDependencies":{"@playwright/test":"1.56.0","turbo":"2"}}`,
		"turbo.jsonc":    "{}",
		"pnpm-lock.yaml": "x", ".gitignore": ignoreMore,
		"apps/web/package.json":               `{"name":"web","dependencies":{"next":"16"}}`,
		"apps/web/test-results/trace.zip":     "x",
		"apps/web/playwright/.auth/user.json": `{"cookies":[]}`,
		".turbo/cache/abc.tar.zst":            "x",
		".turbo/config.json":                  `{"teamid":"x"}`,
	})
	testfix.Repo(t, mono, testfix.Old)
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(mono, "apps", "web", "test-results")), Safe)
	want(t, item(t, res, filepath.Join(mono, ".turbo", "cache")), Safe)
	noItem(t, res, filepath.Join(mono, ".turbo"))
	noItem(t, res, filepath.Join(mono, "apps", "web", "playwright"))
	noItem(t, res, filepath.Join(mono, "apps", "web", "playwright", ".auth"))
}

func TestNeverTouchUserData(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "site")
	testfix.Write(t, app, map[string]string{
		"package.json": pkg(`"nuxt":"4","wrangler":"4","netlify-cli":"23","mastra":"1"`), "package-lock.json": "{}", ".gitignore": ignoreMore,
		"sst.config.ts": "x", "netlify.toml": "x",
		".netlify/state.json": "{}", ".netlify/functions-serve/fn/index.js": "x", ".netlify/db/data": "x",
		".wrangler/state/v3/d1/db.sqlite": "x", ".wrangler/tmp/bundle.js": "x",
		".sst/stage": "prod", ".sst/platform/package.json": "{}",
		".mastra/output/mastra.db": "x", ".mastra/.build/x.js": "x",
		".data/kv/key": "x", ".nitro/dev/index.mjs": "x",
	})
	testfix.Repo(t, app, testfix.Old)
	res := scanFix(t, root)
	for _, ok := range []string{".netlify/functions-serve", ".wrangler/tmp", ".sst/platform", ".mastra/.build", ".nitro"} {
		item(t, res, filepath.Join(app, filepath.FromSlash(ok)))
	}
	for _, never := range []string{".netlify", ".netlify/db", ".wrangler/state", ".wrangler", ".sst", ".sst/stage", ".mastra", ".mastra/output", ".data"} {
		noItem(t, res, filepath.Join(app, filepath.FromSlash(never)))
	}
	// .sst/platform holds a package.json; it must not become a project.
	for _, p := range res.Projects {
		if strings.Contains(p.Path, ".sst") {
			t.Errorf("walked into %s", p.Path)
		}
	}
}

func TestTauriFoundryAndWorktree(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "desk")
	testfix.Write(t, app, map[string]string{
		"package.json": `{"name":"desk","devDependencies":{"@tauri-apps/cli":"2"}}`, "package-lock.json": "{}", ".gitignore": ignoreMore,
		"src-tauri/Cargo.toml": "[package]", "src-tauri/target/debug/desk": "x",
	})
	testfix.Repo(t, app, testfix.Old)
	chain := filepath.Join(root, "chain")
	testfix.Write(t, chain, map[string]string{
		"package.json": `{"name":"chain","devDependencies":{"hardhat":"3"}}`, "package-lock.json": "{}", ".gitignore": ignoreMore,
		"hardhat.config.ts": "x", "foundry.toml": "x", "cache/solidity-files-cache.json": "{}", "out/C.sol/C.json": "{}",
	})
	testfix.Repo(t, chain, testfix.Old)
	// A linked worktree of desk, as agents create them.
	wt := filepath.Join(root, "desk-wt")
	testfix.Git(t, app, testfix.Old, "worktree", "add", "-q", "--detach", wt)
	testfix.Write(t, wt, map[string]string{"node_modules/a/index.js": "x"})
	testfix.Age(t, wt, testfix.Old)
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "src-tauri", "target")), Review)
	// Foundry keeps compiled contracts in out/ and cache/: never offered.
	noItem(t, res, filepath.Join(chain, "cache"))
	noItem(t, res, filepath.Join(chain, "out"))
	want(t, item(t, res, filepath.Join(wt, "node_modules")), Review, ReasonWorktree)
}

func TestUnreadableFoldersAreReported(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{
		"package.json": pkg(`"next":"16"`), "package-lock.json": "{}", ".gitignore": ignoreAll,
		"node_modules/ok/index.js": "x", "node_modules/locked/index.js": "x", "../locked/p/package.json": "{}",
	})
	testfix.Repo(t, app, testfix.Old)
	for _, d := range []string{filepath.Join(app, "node_modules", "locked"), filepath.Join(root, "locked")} {
		os.Chmod(d, 0)
		t.Cleanup(func() { os.Chmod(d, 0o755) })
	}
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "node_modules")), Review, ReasonPartialRead)
	if len(res.Warnings) == 0 || !strings.Contains(strings.Join(res.Warnings, "\n"), "could not be read") {
		t.Errorf("warnings %v", res.Warnings)
	}
}
