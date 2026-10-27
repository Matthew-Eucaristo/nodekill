package scan

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

// fakeEnv answers tool queries from a table; tools not in it are missing.
func fakeEnv(t *testing.T, home string, answers map[string]string) Env {
	bin := t.TempDir()
	return Env{
		Home: home, GOOS: runtime.GOOS, Getenv: func(string) string { return "" },
		LookPath: func(name string) (string, error) {
			for k := range answers {
				if strings.HasPrefix(k, name+" ") || k == name {
					return filepath.Join(bin, name), nil
				}
			}
			return "", errors.New("not found")
		},
		Query: func(_, exe string, args ...string) ([]byte, error) {
			key := filepath.Base(exe) + " " + strings.Join(args, " ")
			for k, v := range answers {
				if strings.HasPrefix(key, k) {
					return []byte(v), nil
				}
			}
			return nil, errors.New("unknown query " + key)
		},
	}
}

func byKind(items []*Item, kind string) []*Item {
	var out []*Item
	for _, it := range items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func TestCachesUseOfficialCommands(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("paths below are the Unix defaults")
	}
	home := testfix.Real(t, t.TempDir())
	npmDir := filepath.Join(home, "npmcache")
	store := filepath.Join(home, "pnpmhome", "store", "v11")
	testfix.Write(t, home, map[string]string{
		"npmcache/_cacache/index-v5/a":                                         "x",
		"npmcache/_npx/0123456789abcdef/package.json":                          `{"_npx":{"packages":["create-next-app@latest"]},"dependencies":{"create-next-app":"16"}}`,
		"npmcache/_npx/0123456789abcdef/node_modules/create-next-app/index.js": "x",
		"npmcache/_npx/not-a-hash/package.json":                                "{}",
		"pnpmhome/store/v11/index.db":                                          "x",
		"pnpmhome/store/v10/files/a":                                           "x",
		".yarn/berry/cache/a.zip":                                              "x",
	})
	env := fakeEnv(t, home, map[string]string{
		"npm config get cache": npmDir, "npm --version": "11.7.0", "pnpm store path": store,
		"npm root -g": filepath.Join(home, "none"), "npm ls -g": `{"dependencies":{}}`, "pnpm ls -g": "[]",
	})
	items, _ := globals(env, &procs.Table{CwdKnown: true})

	npm := byKind(items, "npm-cache")
	if len(npm) != 1 || filepath.Base(npm[0].Command[0]) != "npm" || strings.Join(npm[0].Command[1:4], " ") != "cache clean --force" {
		t.Fatalf("npm cache: %+v", npm)
	}
	npx := byKind(items, "npx")
	if len(npx) != 1 || npx[0].Name != "npx create-next-app@latest" || npx[0].Command[4] != "0123456789abcdef" || npx[0].LastUsedSource != "last run" {
		t.Fatalf("npx: %+v", npx)
	}
	pn := byKind(items, "pnpm-store")
	if len(pn) != 1 || pn[0].Path != store || pn[0].Command[1] != "store" || pn[0].Command[2] != "prune" {
		t.Fatalf("pnpm store: %+v", pn)
	}
	old := byKind(items, "pnpm-store-old")
	if len(old) != 1 || filepath.Base(old[0].Path) != "v10" || len(old[0].Command) != 0 || old[0].Safety.Level != Review {
		t.Fatalf("old pnpm store: %+v", old)
	}
	berry := byKind(items, "yarn-berry-cache")
	if len(berry) != 1 || berry[0].Safety.Level != Protected || !berry[0].Safety.Has(ReasonNotInstalled) || len(berry[0].Command) != 0 {
		t.Fatalf("berry without yarn must be shown, never removed by hand: %+v", berry)
	}
}

func TestToolRunningBlocksCacheCommand(t *testing.T) {
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{"c/_cacache/a": "x"})
	env := fakeEnv(t, home, map[string]string{"npm config get cache": filepath.Join(home, "c"), "npm --version": "11.0.0", "npm root -g": "", "npm ls -g": ""})
	items, _ := globals(env, &procs.Table{CwdKnown: true, Procs: []procs.Proc{{PID: 9, Command: "npm"}}})
	if it := byKind(items, "npm-cache"); len(it) != 1 || it[0].Safety.Level != Live {
		t.Fatalf("%+v", it)
	}
}

func TestGlobalPackages(t *testing.T) {
	home := testfix.Real(t, t.TempDir())
	root := filepath.Join(home, "lib", "node_modules")
	testfix.Write(t, root, map[string]string{
		"npm/package.json":        `{"name":"npm","bin":{"npm":"bin/npm-cli.js"}}`,
		"npm/bin/npm-cli.js":      "x",
		"typescript/package.json": `{"name":"typescript","bin":{"tsc":"bin/tsc"}}`,
		"typescript/bin/tsc":      "x",
	})
	env := fakeEnv(t, home, map[string]string{
		"npm root -g": root, "npm ls -g": `{"dependencies":{"npm":{"version":"11.0.0"},"typescript":{"version":"5.9.0"}}}`, "npm --version": "11.0.0", "npm config get cache": "",
	})
	items, _ := globals(env, &procs.Table{CwdKnown: true})
	for _, it := range byKind(items, "npm-global") {
		switch {
		case strings.HasPrefix(it.Name, "npm@"):
			if it.Safety.Level != Protected || len(it.Command) != 0 {
				t.Errorf("npm itself must never be offered: %+v", it.Safety)
			}
		case strings.HasPrefix(it.Name, "typescript@"):
			if it.Safety.Level != Review || strings.Join(it.Command[1:4], " ") != "uninstall -g typescript" || it.LastUsed.IsZero() {
				t.Errorf("typescript: %+v %v", it.Safety, it.Command)
			}
		default:
			t.Errorf("unexpected %s", it.Name)
		}
	}
}

func TestNvmVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nvm is Unix-only")
	}
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{
		".nvm/nvm.sh": "#", ".nvm/alias/default": "lts/iron", ".nvm/alias/lts/iron": "v20",
		".nvm/versions/node/v20.11.0/bin/node": "x", ".nvm/versions/node/v20.19.5/bin/node": "x",
		".nvm/versions/node/v22.1.0/bin/node": "x",
	})
	env := fakeEnv(t, home, map[string]string{"bash": ""})
	got := map[string]Level{}
	for _, it := range nvmVersions(&tools{env: env, path: map[string]string{}}, "") {
		got[it.Version] = it.Safety.Level
		if it.Shown != "nvm uninstall v"+it.Version {
			t.Errorf("shown %q", it.Shown)
		}
	}
	// The default alias resolves through lts/iron to the newest v20.
	if got["20.19.5"] != Protected || got["20.11.0"] != Review || got["22.1.0"] != Review {
		t.Fatalf("levels %v", got)
	}
}

func TestPinnedVersions(t *testing.T) {
	p := &Project{Name: "web", Path: t.TempDir(), names: map[string]bool{".nvmrc": true}}
	os.WriteFile(filepath.Join(p.Path, ".nvmrc"), []byte("22\n"), 0o644)
	a := &Item{Version: "22.1.0", Safety: Safety{Level: Review}}
	b := &Item{Version: "20.1.0", Safety: Safety{Level: Review}}
	pinned([]*Item{a, b}, []*Project{p})
	if !strings.Contains(a.Note, "pinned by web") || !strings.Contains(b.Note, "no project") {
		t.Fatalf("%q / %q", a.Note, b.Note)
	}
}

func TestNewestAndMatches(t *testing.T) {
	vs := []string{"v18.20.0", "v20.9.0", "v20.11.1", "v22.0.0"}
	if newest(vs, "20") != "v20.11.1" || newest(vs, "") != "v22.0.0" || newest(vs, "lts/x") != "" {
		t.Fatal("newest")
	}
	if !matches("v20", "20.1.0") || matches("2", "20.1.0") || matches("", "1.0.0") {
		t.Fatal("matches")
	}
}
