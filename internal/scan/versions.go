package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// nodeVersions lists Node versions installed by version managers and
// Homebrew. The version in use and each manager's default are protected;
// the rest are offered for review with their manager's own uninstall.
func nodeVersions(t *tools) ([]*Item, []string) {
	cur := currentNode(t)
	var items []*Item
	for _, f := range []func(*tools, string) []*Item{nvmVersions, fnmVersions, miseVersions, nVersions, asdfVersions, nodenvVersions, voltaVersions, nvmWindowsVersions, brewNodes} {
		items = append(items, f(t, cur)...)
	}
	return items, nil
}

// currentNode is the real path of the node on PATH, "" when none.
func currentNode(t *tools) string {
	p := t.look("node")
	if p == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// nodeRuntime builds one installed Node version.
func nodeRuntime(manager, version, dir string, cmd []string, exe, cur string, isDefault bool) *Item {
	it := newGlobal(CatRuntime, "node-version", "Node "+version+" ("+manager+")", manager, dir, cmd, exe)
	it.Explain = "A Node.js version installed by " + manager + ", with any global packages installed into it."
	it.Regenerate = "reinstall it with " + manager + " if a project needs it"
	it.LastUsed, it.LastUsedSource = nodeUsed(dir)
	it.Version = strings.TrimPrefix(version, "v")
	switch {
	case cur != "" && strings.HasPrefix(strings.ToLower(cur), strings.ToLower(dir)+string(filepath.Separator)):
		it.Safety.add(ReasonCurrent, Protected, "the node on your PATH")
	case isDefault:
		it.Safety.add(ReasonCurrent, Protected, manager+"'s default version")
	default:
		it.Safety.add(ReasonCatalogReview, Review, "check that no project still needs this version")
	}
	return it
}

// nodeUsed: running node updates its binary's access time on macOS and
// Linux; otherwise fall back to when the version was installed.
func nodeUsed(dir string) (time.Time, string) {
	for _, bin := range []string{"bin/node", "node.exe", "installation/bin/node"} {
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(bin))); err == nil {
			if at := accessTime(fi); !at.IsZero() {
				return at, "last run"
			}
			return fi.ModTime(), "installed"
		}
	}
	if fi, err := os.Stat(dir); err == nil {
		return fi.ModTime(), "installed"
	}
	return time.Time{}, ""
}

var semver = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

func subdirs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() && semver.MatchString(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// matches reports whether an alias such as "22", "v22.1", "22.1.0" names
// version v. Fail closed: anything else does not match (and is reported).
func matches(alias, v string) bool {
	a, v := strings.TrimPrefix(strings.TrimSpace(alias), "v"), strings.TrimPrefix(v, "v")
	return a != "" && (a == v || strings.HasPrefix(v, a+"."))
}

// ── nvm (macOS, Linux) ──

func nvmVersions(t *tools, cur string) []*Item {
	if t.env.GOOS == "windows" {
		return nil
	}
	dir := t.env.Getenv("NVM_DIR")
	if dir == "" {
		dir = filepath.Join(t.env.Home, ".nvm")
		if x := t.env.Getenv("XDG_CONFIG_HOME"); x != "" && isDir(filepath.Join(x, "nvm")) {
			dir = filepath.Join(x, "nvm")
		}
	}
	versions := subdirs(filepath.Join(dir, "versions", "node"))
	if len(versions) == 0 {
		return nil
	}
	bash, script := t.look("bash"), filepath.Join(dir, "nvm.sh")
	if !fileExists(script) {
		bash = ""
	}
	def := nvmDefault(dir, versions)
	var items []*Item
	for _, v := range versions {
		cmd := []string{"bash", "-c", `export NVM_DIR="$2"; . "$NVM_DIR/nvm.sh" && nvm uninstall "$1"`, "nodekill", v, dir}
		it := nodeRuntime("nvm", v, filepath.Join(dir, "versions", "node", v), cmd, bash, cur, v == def)
		it.Shown = "nvm uninstall " + v
		items = append(items, it)
	}
	return items
}

// nvmDefault resolves alias/default (which may name another alias such as
// lts/iron, or a partial version) to the newest installed match.
func nvmDefault(dir string, versions []string) string {
	alias := "default"
	for range 5 {
		b, err := os.ReadFile(filepath.Join(dir, "alias", filepath.FromSlash(alias)))
		if err != nil {
			break
		}
		alias = strings.TrimSpace(string(b))
		if !strings.Contains(alias, "/") && alias != "node" && alias != "stable" {
			break
		}
		if alias == "node" || alias == "stable" {
			return newest(versions, "")
		}
		if alias == "lts/*" {
			alias = "lts/*" // resolved by the next read of alias/lts/*
		}
	}
	return newest(versions, alias)
}

// newest returns the highest installed version matching prefix ("" = any).
func newest(versions []string, prefix string) string {
	var best string
	var bv [3]int
	for _, v := range versions {
		if prefix != "" && !matches(prefix, v) {
			continue
		}
		m := semver.FindStringSubmatch(v)
		var x [3]int
		for i := range 3 {
			x[i] = atoi(m[i+1])
		}
		if best == "" || slices.Compare(x[:], bv[:]) > 0 {
			best, bv = v, x
		}
	}
	return best
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// ── fnm ──

func fnmVersions(t *tools, cur string) []*Item {
	base := t.env.Getenv("FNM_DIR")
	if base == "" {
		cands := []string{filepath.Join(t.env.Home, ".local", "share", "fnm"), filepath.Join(t.env.Home, ".fnm")}
		if x := t.env.Getenv("XDG_DATA_HOME"); x != "" {
			cands = append([]string{filepath.Join(x, "fnm")}, cands...)
		}
		switch t.env.GOOS {
		case "darwin":
			cands = append(cands, filepath.Join(t.env.Home, "Library", "Application Support", "fnm"))
		case "windows":
			cands = []string{filepath.Join(t.env.Getenv("APPDATA"), "fnm")}
		}
		for _, c := range cands {
			if isDir(filepath.Join(c, "node-versions")) {
				base = c
				break
			}
		}
	}
	if base == "" {
		return nil
	}
	def := ""
	if r, err := filepath.EvalSymlinks(filepath.Join(base, "aliases", "default")); err == nil {
		def = filepath.Base(filepath.Dir(r)) // node-versions/<v>/installation
	}
	exe := t.look("fnm")
	var items []*Item
	for _, v := range subdirs(filepath.Join(base, "node-versions")) {
		items = append(items, nodeRuntime("fnm", v, filepath.Join(base, "node-versions", v), []string{"fnm", "uninstall", v}, exe, cur, v == def))
	}
	return items
}

// ── mise ──

func miseVersions(t *tools, cur string) []*Item {
	exe := t.look("mise")
	if exe == "" {
		return nil
	}
	var list []struct {
		Version     string `json:"version"`
		InstallPath string `json:"install_path"`
		Installed   bool   `json:"installed"`
		Active      bool   `json:"active"`
		Symlinked   string `json:"symlinked_to"`
	}
	if json.Unmarshal([]byte(t.out("mise", "ls", "node", "--json")), &list) != nil {
		return nil
	}
	var items []*Item
	for _, v := range list {
		if !v.Installed || v.Symlinked != "" || !filepath.IsAbs(v.InstallPath) {
			continue
		}
		items = append(items, nodeRuntime("mise", v.Version, v.InstallPath, []string{"mise", "uninstall", "node@" + v.Version}, exe, cur, v.Active))
	}
	return items
}

// ── n ──

func nVersions(t *tools, cur string) []*Item {
	if t.env.GOOS == "windows" {
		return nil
	}
	prefix := t.env.Getenv("N_CACHE_PREFIX")
	if prefix == "" {
		prefix = t.env.Getenv("N_PREFIX")
	}
	if prefix == "" {
		prefix = "/usr/local"
	}
	dir := filepath.Join(prefix, "n", "versions", "node")
	versions := subdirs(dir)
	if len(versions) == 0 {
		return nil
	}
	installed := ""
	if out := t.out("node", "--version"); cur != "" && strings.HasPrefix(cur, filepath.Join(prefix, "bin")) {
		installed = strings.TrimPrefix(out, "v")
	}
	exe := t.look("n")
	var items []*Item
	for _, v := range versions {
		items = append(items, nodeRuntime("n", v, filepath.Join(dir, v), []string{"n", "rm", v}, exe, cur, v == installed))
	}
	return items
}

// ── asdf ──

func asdfVersions(t *tools, cur string) []*Item {
	data := t.env.Getenv("ASDF_DATA_DIR")
	if data == "" {
		data = filepath.Join(t.env.Home, ".asdf")
	}
	dir := filepath.Join(data, "installs", "nodejs")
	versions := subdirs(dir)
	if len(versions) == 0 {
		return nil
	}
	exe := t.look("asdf")
	current := ""
	for _, line := range strings.Split(t.out("asdf", "list", "nodejs"), "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "*") {
			current = strings.TrimSpace(strings.TrimPrefix(l, "*"))
		}
	}
	var items []*Item
	for _, v := range versions {
		items = append(items, nodeRuntime("asdf", v, filepath.Join(dir, v), []string{"asdf", "uninstall", "nodejs", v}, exe, cur, v == current || exe == ""))
	}
	return items
}

// ── nodenv ──

func nodenvVersions(t *tools, cur string) []*Item {
	root := t.env.Getenv("NODENV_ROOT")
	if root == "" {
		root = filepath.Join(t.env.Home, ".nodenv")
	}
	dir := filepath.Join(root, "versions")
	versions := subdirs(dir)
	if len(versions) == 0 {
		return nil
	}
	exe := t.look("nodenv")
	current := t.out("nodenv", "version-name")
	var items []*Item
	for _, v := range versions {
		items = append(items, nodeRuntime("nodenv", v, filepath.Join(dir, v), []string{"nodenv", "uninstall", "-f", v}, exe, cur, v == current || exe == ""))
	}
	return items
}

// ── Volta: no official way to remove a Node version, so report only ──

func voltaVersions(t *tools, cur string) []*Item {
	home := t.env.Getenv("VOLTA_HOME")
	if home == "" {
		home = filepath.Join(t.env.Home, ".volta")
		if t.env.GOOS == "windows" {
			home = filepath.Join(t.env.Getenv("LOCALAPPDATA"), "Volta")
		}
	}
	dir := filepath.Join(home, "tools", "image", "node")
	var items []*Item
	for _, v := range subdirs(dir) {
		it := nodeRuntime("Volta", v, filepath.Join(dir, v), nil, "", cur, false)
		it.Safety = Safety{Level: Safe}
		it.Safety.add(ReasonNoCommand, Protected, "Volta cannot uninstall Node versions and is no longer maintained; its README suggests moving to mise")
		items = append(items, it)
	}
	return items
}

// ── nvm-windows ──

func nvmWindowsVersions(t *tools, cur string) []*Item {
	if t.env.GOOS != "windows" {
		return nil
	}
	exe := t.look("nvm")
	if exe == "" {
		return nil
	}
	root := ""
	for _, line := range strings.Split(t.out("nvm", "root"), "\n") {
		if _, after, ok := strings.Cut(line, "Root:"); ok {
			root = strings.TrimSpace(after)
		}
	}
	current := ""
	for _, line := range strings.Split(t.out("nvm", "list"), "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "*") {
			current = strings.Fields(strings.TrimPrefix(l, "*"))[0]
		}
	}
	if !filepath.IsAbs(root) {
		return nil
	}
	var items []*Item
	for _, v := range subdirs(root) {
		items = append(items, nodeRuntime("nvm-windows", v, filepath.Join(root, v), []string{"nvm", "uninstall", strings.TrimPrefix(v, "v")}, exe, cur, matches(current, v)))
	}
	return items
}

// ── Homebrew node@XX kegs ──

func brewNodes(t *tools, cur string) []*Item {
	exe := t.look("brew")
	if exe == "" || t.env.GOOS == "windows" {
		return nil
	}
	var items []*Item
	for _, line := range strings.Split(t.out("brew", "list", "--formula", "--versions"), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || (f[0] != "node" && !strings.HasPrefix(f[0], "node@")) {
			continue
		}
		cellar := t.out("brew", "--cellar", f[0])
		if !filepath.IsAbs(cellar) || !isDir(cellar) {
			continue
		}
		it := nodeRuntime("Homebrew", f[len(f)-1], cellar, []string{"brew", "uninstall", f[0]}, exe, cur, f[0] == "node")
		it.Name = "Node " + f[len(f)-1] + " (Homebrew " + f[0] + ")"
		it.Note = "global packages live in Homebrew's shared prefix and stay"
		if users := strings.Fields(t.out("brew", "uses", "--installed", f[0])); len(users) > 0 && it.Safety.Level != Protected {
			it.Safety = Safety{Level: Safe}
			it.Safety.add(ReasonDependents, Protected, "needed by "+strings.Join(users, ", "))
		}
		if strings.HasPrefix(strings.ToLower(cur), strings.ToLower(cellar)) {
			it.Safety = Safety{Level: Safe}
			it.Safety.add(ReasonCurrent, Protected, "the node on your PATH")
		}
		items = append(items, it)
	}
	return items
}
