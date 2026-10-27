package scan

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
)

// PM is a package manager.
type PM string

const (
	NPM       PM = "npm"
	PNPM      PM = "pnpm"
	Yarn      PM = "yarn"       // Yarn 1 (classic)
	YarnBerry PM = "yarn-berry" // Yarn 2+
	Bun       PM = "bun"
	Deno      PM = "deno"
)

// Install is the command that brings node_modules back.
func (p PM) Install() string {
	switch p {
	case NPM:
		return "npm ci"
	case PNPM:
		return "pnpm install"
	case Yarn, YarnBerry:
		return "yarn install"
	case Bun:
		return "bun install"
	case Deno:
		return "deno install"
	}
	return "your package manager's install"
}

// lockfiles maps lockfile names to their package manager. yarn.lock is
// classic or berry depending on its content.
var lockfiles = []struct {
	name string
	pm   PM
}{
	{"package-lock.json", NPM}, {"npm-shrinkwrap.json", NPM}, {"pnpm-lock.yaml", PNPM},
	{"yarn.lock", Yarn}, {"bun.lock", Bun}, {"bun.lockb", Bun}, {"deno.lock", Deno},
}

// installRecords are written by each package manager at the end of an
// install; files in node_modules newer than the record were changed by hand
// or by a tool afterwards.
var installRecords = []string{".package-lock.json", ".modules.yaml", ".yarn-state.yml", ".yarn-integrity"}

// packageJSON is the part of package.json nodekill reads.
type packageJSON struct {
	Name            string            `json:"name"`
	PackageManager  string            `json:"packageManager"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Workspaces      json.RawMessage   `json:"workspaces"`
}

// Project is a directory with a package.json.
type Project struct {
	ID   string
	Path string
	Name string
	// Group is the workspace root this project belongs to (itself when it
	// is not a workspace member).
	Group string

	PM        PM
	PMSource  string   // "packageManager", "lockfile", "node_modules" or ""
	Lockfiles []string // lockfile names found here or in the workspace root
	Tools     []string // frameworks and tools in use, for display

	Repo     string // git work tree root; "" outside git
	Worktree bool   // a linked git worktree (fell owns the worktree itself)
	// GitIgnored: git ignores the project folder itself, so it is not
	// source (a deployment bundle, local data, a scratch copy).
	GitIgnored bool

	LastActivity   time.Time
	ActivitySource string // "commit", "files" or ""

	Live     []procs.Proc // node processes working in it
	CwdKnown bool

	Errors []string

	pkg        *packageJSON
	root       *Project        // workspace root, when this is a member
	names      map[string]bool // top-level entries
	workspace  bool            // declares workspaces
	globs      []string        // its workspace patterns; nil with globsKnown false means every subfolder
	globsKnown bool
	newest     time.Time // newest source file seen while walking
}

func (p *Project) has(dep string) bool {
	if p.pkg == nil {
		return false
	}
	_, a := p.pkg.Dependencies[dep]
	_, b := p.pkg.DevDependencies[dep]
	return a || b
}

// usesTool: dep is a dependency here or of the workspace root, where
// monorepos usually hoist test runners and build tools.
func (p *Project) usesTool(dep string) bool {
	return p.has(dep) || (p.root != nil && p.root.has(dep))
}

func (p *Project) hasPrefix(prefix string) bool {
	if p.pkg == nil {
		return false
	}
	for _, m := range []map[string]string{p.pkg.Dependencies, p.pkg.DevDependencies} {
		for k := range m {
			if strings.HasPrefix(k, prefix) {
				return true
			}
		}
	}
	return false
}

// config reports whether a top-level file named base.<ext> exists, e.g.
// config("next.config") for next.config.js/.mjs/.ts.
func (p *Project) config(base string) bool {
	for n := range p.names {
		if n == base || strings.HasPrefix(n, base+".") {
			return true
		}
	}
	return false
}

func (p *Project) file(name string) bool { return p.names[name] }

// configText returns the contents of the first base.* config file, capped,
// for simple checks such as `output: 'standalone'`.
func (p *Project) configText(base string) string {
	for n := range p.names {
		if strings.HasPrefix(n, base+".") {
			b, err := os.ReadFile(filepath.Join(p.Path, n))
			if err == nil && len(b) < 1<<20 {
				return string(b)
			}
		}
	}
	return ""
}

func newProject(dir string, names map[string]bool) *Project {
	p := &Project{ID: id("prj", dir), Path: dir, Name: filepath.Base(dir), Group: dir, names: names}
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		p.Errors = append(p.Errors, "package.json unreadable: "+err.Error())
		return p
	}
	var pkg packageJSON
	if err := json.Unmarshal(b, &pkg); err != nil {
		p.Errors = append(p.Errors, "package.json is not valid JSON")
		return p
	}
	p.pkg = &pkg
	if pkg.Name != "" {
		p.Name = pkg.Name
	}
	ws := strings.TrimSpace(string(pkg.Workspaces))
	p.workspace = (ws != "" && ws != "null" && ws != "[]" && ws != "{}") || names["pnpm-workspace.yaml"]
	switch {
	case names["pnpm-workspace.yaml"]:
		if b, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
			p.globs, p.globsKnown = pnpmPackages(string(b))
		}
	case p.workspace:
		var list []string
		var obj struct{ Packages []string }
		if json.Unmarshal(pkg.Workspaces, &list) == nil {
			p.globs, p.globsKnown = list, true
		} else if json.Unmarshal(pkg.Workspaces, &obj) == nil {
			p.globs, p.globsKnown = obj.Packages, true
		}
	}
	p.Tools = detectTools(p)
	return p
}

// pnpmPackages reads the packages list of pnpm-workspace.yaml. Without one
// pnpm includes every subfolder; a layout it cannot read counts the same.
func pnpmPackages(text string) (globs []string, ok bool) {
	in := false
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		switch {
		case in && strings.HasPrefix(t, "-"):
			globs = append(globs, strings.Trim(strings.TrimSpace(t[1:]), `"'`))
		case line[0] != ' ' && line[0] != '\t':
			if in {
				return globs, true
			}
			if k, v, _ := strings.Cut(t, ":"); k == "packages" {
				switch strings.TrimSpace(v) {
				case "":
					in = true
				default:
					// A flow sequence: packages: ['apps/*', "libs/**"]
					list, ok := strings.CutPrefix(strings.TrimSpace(v), "[")
					if list, ok = strings.CutSuffix(list, "]"); !ok || strings.ContainsAny(list, "[]{}") {
						return nil, false
					}
					for _, g := range strings.Split(list, ",") {
						if g = strings.Trim(strings.TrimSpace(g), `"'`); g != "" {
							globs = append(globs, g)
						}
					}
					return globs, true
				}
			}
		case in:
			return nil, false
		}
	}
	return globs, in
}

// includes reports whether p is one of the workspace's members, by its
// patterns ("apps/*", "packages/**", "!**/test/**").
func (w *Project) includes(p *Project) bool {
	if !w.globsKnown {
		return true
	}
	rel, err := filepath.Rel(w.Path, p.Path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	in := false
	for _, g := range w.globs {
		neg := strings.HasPrefix(g, "!")
		g = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(g, "!"), "./"), "/")
		if globMatch(strings.Split(g, "/"), strings.Split(rel, "/")) {
			if neg {
				return false
			}
			in = true
		}
	}
	return in
}

// globMatch matches path segments; "**" spans any number of them.
func globMatch(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globMatch(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], segs[0])
	return ok && err == nil && globMatch(pat[1:], segs[1:])
}

// detectPM decides the package manager: the packageManager field, else the
// lockfiles here or in the workspace root (members only), else the marker
// node_modules carries. Several different lockfiles leave it ambiguous.
func (p *Project) detectPM(root *Project) {
	p.root = root
	lockRoot := root
	if root != nil && !root.includes(p) {
		lockRoot = nil
	}
	if p.pkg != nil && p.pkg.PackageManager != "" {
		name, _, _ := strings.Cut(p.pkg.PackageManager, "@")
		if pm := PM(name); slices.Contains([]PM{NPM, PNPM, Yarn, Bun}, pm) {
			p.PM, p.PMSource = pm, "packageManager"
			if pm == Yarn && !strings.HasPrefix(strings.TrimPrefix(p.pkg.PackageManager, "yarn@"), "1.") {
				p.PM = YarnBerry
			}
		}
	}
	for _, dir := range []*Project{p, lockRoot} {
		if dir == nil {
			continue
		}
		for _, l := range lockfiles {
			if dir.names[l.name] && !slices.Contains(p.Lockfiles, l.name) {
				p.Lockfiles = append(p.Lockfiles, l.name)
			}
		}
		if len(p.Lockfiles) > 0 {
			break
		}
	}
	if p.PM == "" && len(p.Lockfiles) > 0 {
		pms := map[PM]bool{}
		for _, n := range p.Lockfiles {
			for _, l := range lockfiles {
				if l.name == n {
					pms[l.pm] = true
				}
			}
		}
		if len(pms) == 1 {
			for pm := range pms {
				p.PM, p.PMSource = pm, "lockfile"
			}
		}
	}
	if p.PM == Yarn && p.yarnBerry(lockRoot) {
		p.PM = YarnBerry
	}
	if p.PM == "" {
		nm := filepath.Join(p.Path, "node_modules")
		for marker, pm := range map[string]PM{".package-lock.json": NPM, ".modules.yaml": PNPM, ".yarn-integrity": Yarn, ".yarn-state.yml": YarnBerry, ".bun": Bun} {
			if _, err := os.Lstat(filepath.Join(nm, marker)); err == nil {
				p.PM, p.PMSource = pm, "node_modules"
				break
			}
		}
	}
}

// yarnBerry: berry lockfiles carry a __metadata block; classic ones do not.
func (p *Project) yarnBerry(root *Project) bool {
	for _, dir := range []*Project{p, root} {
		if dir == nil {
			continue
		}
		if dir.names[".yarnrc.yml"] {
			return true
		}
		if f, err := os.Open(filepath.Join(dir.Path, "yarn.lock")); err == nil {
			buf := make([]byte, 4096)
			n, _ := f.Read(buf)
			f.Close()
			if strings.Contains(string(buf[:n]), "__metadata:") {
				return true
			}
		}
	}
	return false
}

// tools, in display order: the dependency or config file that proves each.
var toolCatalog = []struct {
	name    string
	deps    []string
	configs []string
}{
	{"Next.js", []string{"next"}, []string{"next.config"}},
	{"Nuxt", []string{"nuxt"}, []string{"nuxt.config"}},
	{"SvelteKit", []string{"@sveltejs/kit"}, []string{"svelte.config"}},
	{"Astro", []string{"astro"}, []string{"astro.config"}},
	{"Angular", []string{"@angular/core"}, []string{"angular.json"}},
	{"Remix", []string{"@remix-run/dev"}, []string{"remix.config"}},
	{"React Router", []string{"@react-router/dev"}, []string{"react-router.config"}},
	{"TanStack", []string{"@tanstack/react-start", "@tanstack/start", "@tanstack/router-plugin"}, nil},
	{"Expo", []string{"expo"}, nil},
	{"React Native", []string{"react-native"}, nil},
	{"Electron", []string{"electron"}, []string{"electron-builder", "forge.config"}},
	{"Gatsby", []string{"gatsby"}, []string{"gatsby-config"}},
	{"Docusaurus", []string{"@docusaurus/core"}, []string{"docusaurus.config"}},
	{"Remotion", []string{"remotion", "@remotion/cli"}, []string{"remotion.config"}},
	{"Vite", []string{"vite"}, []string{"vite.config"}},
	{"Parcel", []string{"parcel"}, []string{".parcelrc"}},
	{"Turborepo", []string{"turbo"}, []string{"turbo.json"}},
	{"Nx", []string{"nx"}, []string{"nx.json"}},
	{"Storybook", []string{"storybook"}, []string{".storybook"}},
	{"Wrangler", []string{"wrangler"}, []string{"wrangler.toml", "wrangler.json", "wrangler.jsonc"}},
}

func detectTools(p *Project) []string {
	var out []string
	for _, t := range toolCatalog {
		found := false
		for _, d := range t.deps {
			found = found || p.has(d)
		}
		for _, c := range t.configs {
			found = found || p.config(c)
		}
		if found {
			out = append(out, t.name)
		}
	}
	return out
}

// Framework is the first detected tool, for grouping.
func (p *Project) Framework() string {
	if len(p.Tools) == 0 {
		return "Node.js"
	}
	return p.Tools[0]
}
