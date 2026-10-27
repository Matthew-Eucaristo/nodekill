package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
)

// npmQuiet keeps npm from writing a debug log or an update-notifier stamp,
// even for read-only commands, and from colouring output.
var npmQuiet = []string{"--logs-max=0", "--no-update-notifier", "--color=false"}

// tools caches lookups of the Node tools on PATH for one scan.
type tools struct {
	env  Env
	mu   sync.Mutex
	path map[string]string
}

func (t *tools) look(name string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.path[name]; ok {
		return p
	}
	p, err := t.env.LookPath(name)
	if err != nil {
		p = ""
	} else if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	t.path[name] = p
	return p
}

// out runs a read-only query of a tool, returning "" on any failure.
func (t *tools) out(name string, args ...string) string {
	exe := t.look(name)
	if exe == "" {
		return ""
	}
	if name == "npm" {
		args = append(args, npmQuiet...)
	}
	b, err := t.env.Query(t.env.Home, exe, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// globals lists the machine-wide Node footprint: package-manager caches and
// stores, tool caches, global packages, npx entries and Node versions.
func globals(env Env, table *procs.Table) ([]*Item, []string) {
	t := &tools{env: env, path: map[string]string{}}
	var (
		mu    sync.Mutex
		items []*Item
		warns []string
		wg    sync.WaitGroup
	)
	for _, f := range []func(*tools) ([]*Item, []string){
		npmCache, npxEntries, pnpmStore, yarnCaches, bunCache, denoCache, corepackCache, nodeGyp, toolCaches,
		npmGlobals, pnpmGlobals, bunGlobals, nodeVersions,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			its, ws := f(t)
			mu.Lock()
			items, warns = append(items, its...), append(warns, ws...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.SortStableFunc(items, func(a, b *Item) int { return strings.Compare(a.ID, b.ID) })
	for _, it := range items {
		if it.Safety.Level == "" {
			it.Safety.Level = Safe
		}
		if it.Tool != "" && it.Category == CatCache && len(it.Command) > 0 && table.Running(running[it.Tool]...) {
			it.Safety.add(ReasonToolRunning, Live, it.Tool+" is running; its cache cleanup waits until it stops")
		}
		it.fingerprint()
	}
	return items, warns
}

// running are the process names that use a tool's cache.
var running = map[string][]string{
	"npm": {"npm", "npx"}, "pnpm": {"pnpm"}, "yarn": {"yarn"}, "bun": {"bun"}, "deno": {"deno"}, "corepack": {"corepack"},
}

// newGlobal builds a machine-wide item. With exe == "" the owning tool is
// missing: the item is shown, protected, with the command to run.
func newGlobal(cat Category, kind, name, tool, path string, cmd []string, exe string) *Item {
	it := &Item{ID: id("glb", kind+"\x00"+path+"\x00"+strings.Join(cmd, " ")), Kind: kind, Category: cat, Name: name, Tool: tool, Path: path, Safety: Safety{Level: Safe}}
	if len(cmd) > 0 {
		if exe == "" {
			it.Note = "run `" + strings.Join(cmd, " ") + "` once " + tool + " is installed"
			it.Safety.add(ReasonNotInstalled, Protected, tool+" is not installed, so its official cleanup cannot run here")
		} else {
			it.Command = append([]string{exe}, cmd[1:]...)
			if tool == "npm" {
				it.Command = append(it.Command, npmQuiet...)
			}
		}
	}
	return it
}

func isDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir()
}

// under picks the per-OS default location of a cache.
func (t *tools) under(mac, linux, windows string) string {
	env := t.env
	var p string
	switch env.GOOS {
	case "darwin":
		p = mac
	case "windows":
		p = windows
	default:
		p = linux
		if x := env.Getenv("XDG_CACHE_HOME"); x != "" && strings.HasPrefix(p, ".cache/") {
			return filepath.Join(x, strings.TrimPrefix(p, ".cache/"))
		}
	}
	if p == "" {
		return ""
	}
	if env.GOOS == "windows" {
		if la := env.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, filepath.FromSlash(p))
		}
		return ""
	}
	return filepath.Join(env.Home, filepath.FromSlash(p))
}

// validCache refuses tool-reported paths that are not a plain directory
// below home (or the tool's own system prefix), so a misconfigured tool
// cannot point a cleanup at something broad.
func (t *tools) validCache(p string) bool {
	if p == "" || !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return false
	}
	home := t.env.Home
	if p == home || strings.Count(strings.TrimPrefix(p, home), string(filepath.Separator)) < 2 && strings.HasPrefix(p, home) {
		return false // home itself or a direct child of it
	}
	return isDir(p)
}

// ── package-manager caches ───────────────────────────────────────────────

func npmCacheDir(t *tools) string {
	if d := t.out("npm", "config", "get", "cache"); filepath.IsAbs(d) {
		return d
	}
	if t.env.GOOS == "windows" {
		return t.under("", "", "npm-cache")
	}
	return filepath.Join(t.env.Home, ".npm")
}

func npmCache(t *tools) ([]*Item, []string) {
	dir := filepath.Join(npmCacheDir(t), "_cacache")
	if !isDir(dir) {
		return nil, nil
	}
	it := newGlobal(CatCache, "npm-cache", "npm cache", "npm", dir, []string{"npm", "cache", "clean", "--force"}, t.look("npm"))
	it.Explain = "npm's download cache (_cacache). npm never shrinks it by itself."
	it.Regenerate = "packages are downloaded again on the next install"
	return []*Item{it}, nil
}

// npxEntries lists each package set npx installed for one-off runs; each is
// a folder named by a hash, removed with `npm cache npx rm <hash>`.
func npxEntries(t *tools) ([]*Item, []string) {
	root := filepath.Join(npmCacheDir(t), "_npx")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil
	}
	exe := t.look("npm")
	major, minor, patch := npmVersion(t)
	// npm 11.6.1+ takes a lock in the entry on every run, so the folder's
	// modification time is when it last ran; before that, when installed.
	locks := major > 11 || (major == 11 && (minor > 6 || (minor == 6 && patch >= 1)))
	var items []*Item
	for _, e := range entries {
		if !e.IsDir() || !npxHash.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		names := npxPackages(dir)
		name := "npx " + strings.Join(names, ", ")
		if len(names) == 0 {
			name = "npx " + e.Name()
		}
		it := newGlobal(CatGlobal, "npx", name, "npm", dir, []string{"npm", "cache", "npx", "rm", e.Name()}, exe)
		if exe != "" && major < 11 {
			it.Command = nil
			it.Note = "this npm has no `npm cache npx rm`; the entry moves to the " + "Trash instead"
		}
		it.Explain = "Packages npx downloaded to run a command once. npx downloads them again when needed."
		it.Regenerate = "npx downloads it again on the next run"
		if fi, err := os.Stat(dir); err == nil {
			it.LastUsed, it.LastUsedSource = fi.ModTime(), "installed"
			if locks && exe != "" {
				it.LastUsedSource = "last run"
			}
		}
		if fi, err := os.Stat(filepath.Join(dir, "concurrency.lock")); err == nil && time.Since(fi.ModTime()) < time.Minute {
			it.Safety.add(ReasonInUse, Live, "an npx run is using it right now")
		}
		items = append(items, it)
	}
	return items, nil
}

var npxHash = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

func npmVersion(t *tools) (int, int, int) {
	var v [3]int
	for i, part := range strings.SplitN(t.out("npm", "--version"), ".", 3) {
		v[i], _ = strconv.Atoi(strings.TrimFunc(part, func(r rune) bool { return r < '0' || r > '9' }))
	}
	return v[0], v[1], v[2]
}

// npxPackages names an entry by the specs npx was asked for.
func npxPackages(dir string) []string {
	var pj struct {
		Npx struct {
			Packages []string `json:"packages"`
		} `json:"_npx"`
		Dependencies map[string]string `json:"dependencies"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || json.Unmarshal(b, &pj) != nil {
		return nil
	}
	if len(pj.Npx.Packages) > 0 {
		return pj.Npx.Packages
	}
	var names []string
	for n := range pj.Dependencies {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

func pnpmStore(t *tools) ([]*Item, []string) {
	exe := t.look("pnpm")
	store := t.out("pnpm", "store", "path")
	if store == "" {
		for _, p := range []string{t.under("Library/pnpm/store", ".local/share/pnpm/store", "pnpm/store"), filepath.Join(t.env.Home, ".pnpm-store")} {
			if isDir(p) {
				store = p
			}
		}
		if store == "" {
			return nil, nil
		}
	}
	if !t.validCache(store) {
		if exe == "" {
			// Without pnpm, show the newest store version found.
			vs, _ := filepath.Glob(filepath.Join(store, "v*"))
			if len(vs) == 0 {
				return nil, nil
			}
			store = vs[len(vs)-1]
		} else {
			return nil, []string{"pnpm reported a store path nodekill does not trust; skipped"}
		}
	}
	it := newGlobal(CatCache, "pnpm-store", "pnpm store", "pnpm", store, []string{"pnpm", "store", "prune"}, exe)
	it.Explain = "pnpm's content-addressable store. `pnpm store prune` removes only packages no project on this machine references."
	it.Regenerate = "pruned packages are downloaded again when a project needs them"
	it.Note = "prune frees at most this much; packages still referenced stay, and their project node_modules share blocks with the store"
	items := []*Item{it}
	// Store folders of older pnpm versions (v3, v10 next to v11) are no
	// longer read by the installed pnpm, and no pnpm command removes them.
	if exe != "" {
		cur := filepath.Base(store)
		olds, _ := filepath.Glob(filepath.Join(filepath.Dir(store), "v*"))
		for _, old := range olds {
			if b := filepath.Base(old); b != cur && regexp.MustCompile(`^v\d+$`).MatchString(b) && isDir(old) && versionLess(b, cur) {
				o := newGlobal(CatCache, "pnpm-store-old", "pnpm store "+b+" (old format)", "pnpm", old, nil, exe)
				o.Explain = "A store written by an older pnpm. The installed pnpm uses " + cur + " and never reads it; no pnpm command removes it, so it moves to the Trash."
				o.Regenerate = "only an older pnpm would use it again"
				o.Safety.add(ReasonCatalogReview, Review, "no official command exists for old store versions")
				items = append(items, o)
			}
		}
	}
	return items, nil
}

func versionLess(a, b string) bool {
	x, _ := strconv.Atoi(strings.TrimPrefix(a, "v"))
	y, _ := strconv.Atoi(strings.TrimPrefix(b, "v"))
	return x < y
}

func yarnCaches(t *tools) ([]*Item, []string) {
	exe := t.look("yarn")
	ver := t.out("yarn", "--version")
	var items []*Item
	// Yarn 1 (classic).
	classic := t.under("Library/Caches/Yarn", ".cache/yarn", "Yarn/Cache")
	if strings.HasPrefix(ver, "1.") {
		if d := t.out("yarn", "cache", "dir"); filepath.IsAbs(d) {
			classic = d
		}
	}
	if isDir(classic) {
		cexe := exe
		if !strings.HasPrefix(ver, "1.") {
			cexe = "" // the yarn on PATH is not Yarn 1, which owns this cache
		}
		it := newGlobal(CatCache, "yarn-cache", "Yarn 1 cache", "yarn", classic, []string{"yarn", "cache", "clean"}, cexe)
		it.Explain = "Yarn 1's package cache."
		it.Regenerate = "packages are downloaded again on the next yarn install"
		items = append(items, it)
	}
	// Yarn 2+ (berry): the shared global cache, by default in the global folder.
	gf := filepath.Join(t.env.Home, ".yarn", "berry")
	if x := t.env.Getenv("XDG_DATA_HOME"); x != "" && t.env.GOOS != "windows" {
		gf = filepath.Join(x, "yarn", "berry")
	}
	if t.env.GOOS == "windows" {
		gf = t.under("", "", "Yarn/Berry")
	}
	if g := t.env.Getenv("YARN_GLOBAL_FOLDER"); g != "" {
		gf = g
	}
	if berry := filepath.Join(gf, "cache"); isDir(berry) {
		bexe := exe
		if ver == "" || strings.HasPrefix(ver, "1.") {
			bexe = ""
		}
		it := newGlobal(CatCache, "yarn-berry-cache", "Yarn 2+ global cache", "yarn", berry, []string{"yarn", "cache", "clean", "--all"}, bexe)
		it.Explain = "The cache Yarn 2+ projects share (enableGlobalCache). Projects that commit .yarn/cache are not affected."
		it.Regenerate = "packages are downloaded again on the next yarn install"
		if bexe == "" && exe != "" {
			it.Note = "the yarn on PATH is Yarn 1; run `yarn cache clean --all` inside a Yarn 2+ project"
		}
		items = append(items, it)
	}
	return items, nil
}

func bunCache(t *tools) ([]*Item, []string) {
	exe := t.look("bun")
	dir := t.out("bun", "pm", "cache")
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(t.env.Home, ".bun", "install", "cache")
		if d := t.env.Getenv("BUN_INSTALL_CACHE_DIR"); d != "" {
			dir = d
		}
	}
	if !isDir(dir) {
		return nil, nil
	}
	it := newGlobal(CatCache, "bun-cache", "Bun cache", "bun", dir, []string{"bun", "pm", "cache", "rm"}, exe)
	it.Explain = "Bun's global package cache. On macOS project node_modules are clones of it, so it often frees more than it seems."
	it.Regenerate = "packages are downloaded again on the next bun install"
	if bunGlobalStore(t) {
		it.Safety.add(ReasonSharedStore, Review, "bunfig sets globalStore: every project using it must run bun install again")
	}
	return []*Item{it}, nil
}

func bunGlobalStore(t *tools) bool {
	for _, p := range []string{filepath.Join(t.env.Home, ".bunfig.toml"), filepath.Join(t.env.Getenv("XDG_CONFIG_HOME"), ".bunfig.toml")} {
		if b, err := os.ReadFile(p); err == nil && regexp.MustCompile(`(?m)^\s*globalStore\s*=\s*true`).Match(b) {
			return true
		}
	}
	return false
}

func denoCache(t *tools) ([]*Item, []string) {
	exe := t.look("deno")
	var dir string
	if out := t.out("deno", "info", "--json"); out != "" {
		var info struct {
			DenoDir string `json:"denoDir"`
		}
		if json.Unmarshal([]byte(out), &info) == nil {
			dir = info.DenoDir
		}
	}
	if dir == "" {
		dir = t.env.Getenv("DENO_DIR")
	}
	if dir == "" {
		dir = t.under("Library/Caches/deno", ".cache/deno", "deno")
	}
	if !isDir(dir) {
		return nil, nil
	}
	it := newGlobal(CatCache, "deno-cache", "Deno cache", "deno", dir, []string{"deno", "clean"}, exe)
	it.Explain = "Deno's module cache (DENO_DIR): remote modules, npm packages and compiled output."
	it.Regenerate = "modules are downloaded again on the next run"
	if t.env.Getenv("DENO_REPL_HISTORY") == "" {
		it.Safety.add(ReasonCatalogReview, Review, "deno clean also deletes your REPL history (deno_history.txt), which Deno keeps in this folder")
	}
	return []*Item{it}, nil
}

func corepackCache(t *tools) ([]*Item, []string) {
	dir := t.env.Getenv("COREPACK_HOME")
	if dir == "" {
		dir = t.under(".cache/node/corepack", ".cache/node/corepack", "node/corepack")
	}
	if !isDir(dir) {
		return nil, nil
	}
	it := newGlobal(CatCache, "corepack-cache", "Corepack cache", "corepack", dir, []string{"corepack", "cache", "clean"}, t.look("corepack"))
	it.Explain = "Package-manager versions Corepack downloaded (Node 25+ no longer ships Corepack)."
	it.Regenerate = "Corepack downloads a version again when a project asks for it"
	return []*Item{it}, nil
}

// nodeGyp lists the Node headers node-gyp keeps per Node version and
// removes them with `node-gyp remove <version>`, run through the copy of
// node-gyp npm bundles (nothing is downloaded).
func nodeGyp(t *tools) ([]*Item, []string) {
	dir := t.env.Getenv("npm_config_devdir")
	if dir == "" {
		dir = t.under("Library/Caches/node-gyp", ".cache/node-gyp", "node-gyp/Cache")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	node, script := t.look("node"), ""
	if root := t.out("npm", "root", "-g"); filepath.IsAbs(root) {
		if p := filepath.Join(root, "npm", "node_modules", "node-gyp", "bin", "node-gyp.js"); fileExists(p) {
			script = p
		}
	}
	var items []*Item
	for _, e := range entries {
		if !e.IsDir() || !nodeVersion.MatchString(e.Name()) {
			continue
		}
		var cmd []string
		if node != "" && script != "" {
			cmd = []string{"node", script, "remove", e.Name()}
		}
		it := newGlobal(CatCache, "node-gyp", "node-gyp headers for Node "+e.Name(), "node-gyp", filepath.Join(dir, e.Name()), cmd, node)
		it.Explain = "Node.js headers node-gyp downloaded to build native modules for this Node version."
		it.Regenerate = "downloaded again on the next native build for this version"
		items = append(items, it)
	}
	return items, nil
}

var nodeVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// toolCaches are caches whose tools have no cleanup command of their own;
// they are plain download caches and move to the Trash.
func toolCaches(t *tools) ([]*Item, []string) {
	defs := []struct {
		kind, name, mac, linux, win, explain, regen string
		review                                      string
	}{
		{"typescript-cache", "TypeScript typings cache", "Library/Caches/typescript", ".cache/typescript", "Microsoft/TypeScript",
			"Type definitions your editor's TypeScript server downloads for plain JavaScript files.", "your editor downloads them again when needed", ""},
		{"electron-cache", "Electron downloads", "Library/Caches/electron", ".cache/electron", "electron/Cache",
			"Electron release zips that electron and @electron/get download.", "downloaded again on the next install of electron", ""},
		{"electron-builder-cache", "electron-builder toolsets", "Library/Caches/electron-builder", ".cache/electron-builder", "electron-builder/Cache",
			"Toolsets electron-builder downloads to package apps (NSIS, AppImage, code signing).", "downloaded again on the next packaging run", ""},
		{"playwright-browsers", "Playwright browsers", "Library/Caches/ms-playwright", ".cache/ms-playwright", "ms-playwright",
			"Browsers Playwright downloaded. Playwright removes versions no installed client needs; `npx playwright uninstall --all` removes the rest.",
			"npx playwright install", "browsers download again only when a project runs `npx playwright install`"},
		{"cypress-binaries", "Cypress binaries", "Library/Caches/Cypress", ".cache/Cypress", "Cypress/Cache",
			"Cypress app binaries, one per version (`npx cypress cache prune` keeps the current one).", "npx cypress install",
			"Cypress downloads its binary again on the next install"},
		{"puppeteer-browsers", "Puppeteer browsers", ".cache/puppeteer", ".cache/puppeteer", "",
			"Browsers Puppeteer downloaded (`npx @puppeteer/browsers clear` empties the same folder).", "npx puppeteer browsers install", ""},
	}
	var items []*Item
	for _, d := range defs {
		p := t.under(d.mac, d.linux, d.win)
		if d.kind == "puppeteer-browsers" && t.env.GOOS != "windows" {
			p = filepath.Join(t.env.Home, ".cache", "puppeteer")
		}
		if p == "" || !isDir(p) {
			continue
		}
		it := newGlobal(CatCache, d.kind, d.name, "", p, nil, "")
		it.Explain, it.Regenerate = d.explain, d.regen
		if d.review != "" {
			it.Safety.add(ReasonCatalogReview, Review, d.review)
		}
		items = append(items, it)
	}
	return items, nil
}

// ── global packages ──────────────────────────────────────────────────────

// bundled ship with Node itself; they are never offered.
var bundled = map[string]bool{"npm": true, "corepack": true}

type lsTree struct {
	Path         string `json:"path"`
	Dependencies map[string]struct {
		Version string `json:"version"`
		Path    string `json:"path"`
	} `json:"dependencies"`
}

func npmGlobals(t *tools) ([]*Item, []string) {
	exe := t.look("npm")
	if exe == "" {
		return nil, nil
	}
	root := t.out("npm", "root", "-g")
	out := t.out("npm", "ls", "-g", "--depth=0", "--json")
	var tree lsTree
	if !filepath.IsAbs(root) || json.Unmarshal([]byte(out), &tree) != nil {
		return nil, []string{"could not list npm's global packages"}
	}
	var items []*Item
	for name, d := range tree.Dependencies {
		path := filepath.Join(root, filepath.FromSlash(name))
		it := newGlobal(CatGlobal, "npm-global", name+"@"+d.Version, "npm", path, []string{"npm", "uninstall", "-g", name}, exe)
		globalPackage(it, name, path)
		items = append(items, it)
	}
	return items, nil
}

func pnpmGlobals(t *tools) ([]*Item, []string) {
	exe := t.look("pnpm")
	if exe == "" {
		return nil, nil
	}
	out := t.out("pnpm", "ls", "-g", "--json", "--depth=0")
	var trees []lsTree
	if json.Unmarshal([]byte(out), &trees) != nil {
		return nil, nil
	}
	var items []*Item
	for _, tr := range trees {
		for name, d := range tr.Dependencies {
			path := d.Path
			if path == "" {
				path = filepath.Join(tr.Path, "node_modules", filepath.FromSlash(name))
			}
			it := newGlobal(CatGlobal, "pnpm-global", name+"@"+d.Version, "pnpm", path, []string{"pnpm", "remove", "-g", name}, exe)
			globalPackage(it, name, path)
			items = append(items, it)
		}
	}
	return items, nil
}

func bunGlobals(t *tools) ([]*Item, []string) {
	exe := t.look("bun")
	dir := filepath.Join(t.env.Home, ".bun", "install", "global")
	if d := t.env.Getenv("BUN_INSTALL_GLOBAL_DIR"); d != "" {
		dir = d
	}
	var pj struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || json.Unmarshal(b, &pj) != nil {
		return nil, nil
	}
	var items []*Item
	for name, v := range pj.Dependencies {
		path := filepath.Join(dir, "node_modules", filepath.FromSlash(name))
		it := newGlobal(CatGlobal, "bun-global", name+"@"+strings.TrimPrefix(v, "^"), "bun", path, []string{"bun", "remove", "-g", name}, exe)
		globalPackage(it, name, path)
		items = append(items, it)
	}
	return items, nil
}

// globalPackage fills in what a global package is and when it last ran.
func globalPackage(it *Item, name, path string) {
	it.Explain = "A globally installed package and its commands."
	it.Regenerate = "reinstall it globally if you need it again"
	it.LastUsed, it.LastUsedSource = binUsed(path)
	if bundled[name] {
		it.Safety.add(ReasonCurrent, Protected, name+" ships with Node.js")
		it.Command = nil
		return
	}
	it.Safety.add(ReasonCatalogReview, Review, "a tool you installed on purpose; check when it last ran")
}

// binUsed reads the access time of a package's command files: running a
// global command reads its entry script. On filesystems that do not keep
// access times (noatime, Windows by default) this is the install time.
func binUsed(pkg string) (time.Time, string) {
	var pj struct {
		Bin json.RawMessage `json:"bin"`
	}
	b, err := os.ReadFile(filepath.Join(pkg, "package.json"))
	if err != nil {
		return time.Time{}, ""
	}
	_ = json.Unmarshal(b, &pj)
	var bins []string
	var one string
	var many map[string]string
	if json.Unmarshal(pj.Bin, &one) == nil && one != "" {
		bins = append(bins, one)
	} else if json.Unmarshal(pj.Bin, &many) == nil {
		for _, v := range many {
			bins = append(bins, v)
		}
	}
	var newest time.Time
	src := "installed"
	for _, rel := range bins {
		if fi, err := os.Stat(filepath.Join(pkg, filepath.FromSlash(rel))); err == nil {
			if at := accessTime(fi); at.After(newest) {
				newest, src = at, "last run"
			}
		}
	}
	if fi, err := os.Stat(filepath.Join(pkg, "package.json")); err == nil && (newest.IsZero() || fi.ModTime().After(newest)) {
		if newest.IsZero() {
			newest, src = fi.ModTime(), "installed"
		}
	}
	return newest, src
}
