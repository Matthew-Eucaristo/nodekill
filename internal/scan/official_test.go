package scan

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

// envWith is fakeEnv with environment variables.
func envWith(t *testing.T, home string, answers, vars map[string]string) Env {
	env := fakeEnv(t, home, answers)
	env.Getenv = func(k string) string { return vars[k] }
	return env
}

func scanGlobals(env Env) []*Item {
	items, _ := globals(env, &procs.Table{CwdKnown: true})
	return items
}

func one(t *testing.T, items []*Item, kind string) *Item {
	t.Helper()
	got := byKind(items, kind)
	if len(got) != 1 {
		t.Fatalf("%s: want 1 item, got %d", kind, len(got))
	}
	return got[0]
}

func unixOnly(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("Unix default paths")
	}
}

func TestDeepInHome(t *testing.T) {
	unixOnly(t)
	home := filepath.FromSlash("/h/me")
	for p, want := range map[string]bool{
		"/h/me": false, "/h/me/.npm": false, "/h/me/.npm/_cacache": true, "/h": false, "/other/a/b": false,
		"/h/me/../me/x/y": false, "relative/x": false, "/h/me/Library/Caches/Yarn": true,
	} {
		if got := DeepInHome(filepath.FromSlash(p), home); got != want {
			t.Errorf("DeepInHome(%s) = %v", p, got)
		}
	}
}

func TestCommandsRunFromHomeAndWipesStayDeep(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{"npmcache/_cacache/a": "x"})
	// A cache configured one level below home would make `npm cache clean`
	// empty a top-level folder of it: shown, never run.
	env := fakeEnv(t, home, map[string]string{"npm config get cache": home, "npm --version": "11.7.0", "npm root -g": "", "npm ls -g": ""})
	testfix.Write(t, home, map[string]string{"_cacache/a": "x"})
	it := one(t, scanGlobals(env), "npm-cache")
	if it.CommandDir != home || it.Safety.Level != Protected || !it.Safety.Has(ReasonBroadPath) {
		t.Fatalf("broad npm cache: dir=%q %+v", it.CommandDir, it.Safety)
	}
	env = fakeEnv(t, home, map[string]string{"npm config get cache": filepath.Join(home, "npmcache"), "npm --version": "11.7.0", "npm root -g": "", "npm ls -g": ""})
	if it := one(t, scanGlobals(env), "npm-cache"); it.CommandDir != home || it.Safety.Level != Safe {
		t.Fatalf("npm cache: dir=%q %+v", it.CommandDir, it.Safety)
	}
	if !strings.Contains(it.Explain, "npm cache verify") {
		t.Error("npm's own advice is missing")
	}
}

func TestNpxRmNeedsNpm112(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{"c/_npx/0123456789abcdef/package.json": `{"_npx":{"packages":["cowsay"]}}`})
	for v, want := range map[string]bool{"11.1.0": false, "11.2.0": true, "10.9.4": false, "12.0.0": true} {
		env := fakeEnv(t, home, map[string]string{"npm config get cache": filepath.Join(home, "c"), "npm --version": v, "npm root -g": "", "npm ls -g": ""})
		if it := one(t, scanGlobals(env), "npx"); (len(it.Command) > 0) != want {
			t.Errorf("npm %s: command %v", v, it.Command)
		}
	}
}

func TestYarnCaches(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	root := filepath.Join(home, "ycache")
	berry := filepath.Join(home, ".yarn", "berry", "cache")
	testfix.Write(t, home, map[string]string{"ycache/v6/a": "x", ".yarn/berry/cache/a.zip": "x"})
	// Yarn 1: the item is the cache root that `yarn cache clean` empties.
	env := envWith(t, home, map[string]string{"yarn --version": "1.22.22", "yarn cache dir": filepath.Join(root, "v6")}, map[string]string{"YARN_CACHE_FOLDER": root})
	items := scanGlobals(env)
	if it := one(t, items, "yarn-cache"); it.Path != root || strings.Join(it.Command[1:], " ") != "cache clean" {
		t.Fatalf("yarn 1: %s %v", it.Path, it.Command)
	}
	if it := one(t, items, "yarn-berry-cache"); it.Safety.Level != Protected || len(it.Command) != 0 {
		t.Fatalf("berry with Yarn 1 on PATH must not run: %+v", it.Safety)
	}
	// Yarn 4: runs only when cacheFolder, as seen from home, is this cache.
	env = fakeEnv(t, home, map[string]string{"yarn --version": "4.9.1", "yarn config get cacheFolder": berry})
	if it := one(t, scanGlobals(env), "yarn-berry-cache"); it.Safety.Level != Safe || strings.Join(it.Command[1:], " ") != "cache clean" || it.CommandDir != home {
		t.Fatalf("berry: %+v %v", it.Safety, it.Command)
	}
	env = fakeEnv(t, home, map[string]string{"yarn --version": "4.9.1", "yarn config get cacheFolder": filepath.Join(home, ".yarn", "cache")})
	if it := one(t, scanGlobals(env), "yarn-berry-cache"); it.Safety.Level != Protected || !it.Safety.Has(ReasonUnknownFormat) {
		t.Fatalf("berry with another cacheFolder must not run: %+v", it.Safety)
	}
}

func TestBunCacheNamesItsFolder(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{"bunhome/install/cache/a": "x"})
	env := envWith(t, home, map[string]string{"bun pm cache": "not a path"}, map[string]string{"BUN_INSTALL": filepath.Join(home, "bunhome")})
	it := one(t, scanGlobals(env), "bun-cache")
	want := filepath.Join(home, "bunhome", "install", "cache")
	if it.Path != want || !slices.Equal(it.Command[1:], []string{"pm", "cache", "rm", "--cache-dir", want}) {
		t.Fatalf("bun: %s %v", it.Path, it.Command)
	}
}

func TestDenoCleanProtections(t *testing.T) {
	unixOnly(t)
	cases := map[string]struct {
		files map[string]string
		want  Level
		code  string
	}{
		"plain":         {map[string]string{"c/dd/remote/a": "x"}, Review, ReasonCatalogReview},
		"location data": {map[string]string{"c/dd/location_data/abc/local_storage": "x"}, Protected, ReasonCatalog},
		"install root":  {map[string]string{"c/dd/bin/deno": "x"}, Protected, ReasonBroadPath},
	}
	for name, c := range cases {
		home := testfix.Real(t, t.TempDir())
		testfix.Write(t, home, c.files)
		env := envWith(t, home, map[string]string{"deno info --json": `{"denoDir":"` + filepath.Join(home, "c", "dd") + `"}`}, nil)
		it := one(t, scanGlobals(env), "deno-cache")
		if it.Safety.Level != c.want || !it.Safety.Has(c.code) {
			t.Errorf("%s: %+v", name, it.Safety)
		}
	}
}

func TestCorepack(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{".cache/node/corepack/v1/yarn/4.0.0/a": "x", ".cache/node/corepack/lastKnownGood.json": "{}"})
	env := fakeEnv(t, home, map[string]string{"corepack --version": "0.34.0"})
	it := one(t, scanGlobals(env), "corepack-cache")
	if filepath.Base(it.Path) != "v1" || len(it.Command) == 0 || it.Safety.Level != Safe {
		t.Fatalf("corepack: %s %v %+v", it.Path, it.Command, it.Safety)
	}
	env = fakeEnv(t, home, map[string]string{"corepack --version": "0.24.1"})
	if it := one(t, scanGlobals(env), "corepack-cache"); len(it.Command) != 0 || it.Safety.Level != Protected {
		t.Fatalf("old corepack must not run: %v", it.Command)
	}
}

func TestDenoAndYarnGlobals(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{
		".deno/bin/deno":                   "\x7fELF binary",
		".deno/bin/fresh":                  "#!/bin/sh\n# generated by deno install\nexec deno run --allow-all 'jsr:@fresh/init' \"$@\"\n",
		".deno/bin/other":                  "#!/bin/sh\necho mine\n",
		".config/yarn/global/package.json": `{"dependencies":{"serve":"^14.2.0"}}`,
		".config/yarn/global/node_modules/serve/package.json": `{"name":"serve","bin":{"serve":"build/main.js"}}`,
	})
	env := fakeEnv(t, home, map[string]string{"deno --version": "deno 2.5.1 (stable)", "yarn --version": "1.22.22"})
	items := scanGlobals(env)
	deno := one(t, items, "deno-global")
	if deno.Name != "fresh (deno install)" || strings.Join(deno.Command[1:], " ") != "uninstall -g fresh" || deno.Safety.Level != Review {
		t.Fatalf("deno global: %+v %v", deno.Name, deno.Command)
	}
	if y := one(t, items, "yarn-global"); strings.Join(y.Command[1:], " ") != "global remove serve" {
		t.Fatalf("yarn global: %v", y.Command)
	}
	env = fakeEnv(t, home, map[string]string{"deno --version": "deno 1.46.3", "yarn --version": "4.9.1"})
	items = scanGlobals(env)
	if it := one(t, items, "deno-global"); len(it.Command) != 0 {
		t.Error("deno 1.x has a different uninstall; it must not run")
	}
	if it := one(t, items, "yarn-global"); len(it.Command) != 0 || !strings.Contains(it.Note, "not Yarn 1") || !strings.Contains(it.Safety.Reasons[0].Detail, "Yarn 1 is not on PATH") {
		t.Errorf("yarn global needs Yarn 1, and must say so: %q %+v", it.Note, it.Safety.Reasons)
	}
}

func TestReportOnlyAndMissingManagers(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{
		".nvs/node/20.11.0/arm64/bin/node":                 "x",
		".proto/tools/node/22.1.0/bin/node":                "x",
		".local/share/mise/installs/node/18.20.0/bin/node": "x",
	})
	items, _ := nodeVersions(&tools{env: fakeEnv(t, home, nil), path: map[string]string{}})
	got := map[string]*Item{}
	for _, it := range items {
		got[it.Tool] = it
	}
	for _, m := range []string{"nvs", "proto"} {
		if it := got[m]; it == nil || it.Safety.Level != Protected || !it.Safety.Has(ReasonNoCommand) || !strings.Contains(it.Note, "`") {
			t.Errorf("%s: %+v", m, it)
		}
	}
	if it := got["mise"]; it == nil || it.Safety.Level != Protected || !it.Safety.Has(ReasonNotInstalled) {
		t.Errorf("mise without mise: %+v", it)
	}
}

func TestNvmFromHomebrewAndCache(t *testing.T) {
	unixOnly(t)
	home := testfix.Real(t, t.TempDir())
	testfix.Write(t, home, map[string]string{".nvm/nvm.sh": "#", ".nvm/.cache/bin/a.tar.xz": "x", ".nvm/versions/node/v20.1.0/bin/node": "x"})
	env := fakeEnv(t, home, map[string]string{"bash": ""})
	items := nvmVersions(&tools{env: env, path: map[string]string{}}, "")
	var cache *Item
	for _, it := range items {
		if it.Kind == "nvm-cache" {
			cache = it
		}
	}
	if cache == nil || cache.Shown != "nvm cache clear" || cache.Command[len(cache.Command)-1] != "clear" {
		t.Fatalf("nvm cache: %+v", cache)
	}
	// An alias that names no installed version is reported on each version.
	testfix.Write(t, home, map[string]string{".nvm/alias/default": "16"})
	for _, it := range nvmVersions(&tools{env: env, path: map[string]string{}}, "") {
		if it.Kind == "node-version" && !strings.Contains(it.Note, "could not resolve") {
			t.Errorf("unresolved default not reported: %q", it.Note)
		}
	}
}

func TestQuietEnvBlocksDownloads(t *testing.T) {
	for _, want := range []string{"COREPACK_ENABLE_NETWORK=0", "pnpm_config_pm_on_fail=ignore", "HOMEBREW_NO_AUTOREMOVE=1", "HOMEBREW_NO_AUTO_UPDATE=1"} {
		if !slices.Contains(QuietEnv, want) {
			t.Errorf("QuietEnv lacks %s", want)
		}
	}
}
