// Package scan finds JavaScript projects and the rebuildable directories,
// caches and installs around them, and decides how freely each may go. It
// only reads; all removal lives in internal/clean.
package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/gitx"
	"github.com/Matthew-Eucaristo/nodekill/internal/pathx"
	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
	"github.com/Matthew-Eucaristo/nodekill/internal/size"
)

// Category groups items in the UI and in JSON.
type Category string

const (
	CatProject Category = "project" // inside a project: node_modules, .next, …
	CatCache   Category = "cache"   // a package manager's or tool's global cache
	CatGlobal  Category = "global"  // globally installed packages and npx entries
	CatRuntime Category = "runtime" // installed Node versions
)

// Item is one thing nodekill can remove.
type Item struct {
	ID          string
	Fingerprint string
	Kind        string
	Category    Category
	Name        string
	Path        string
	Project     *Project // nil outside projects
	Rule        *Rule    // project items only
	Tool        string   // owning tool for caches, globals and runtimes
	// Command is the owning tool's official removal command. Items with a
	// command are removed by running it (which deletes permanently, by the
	// tool's design); items without one move to the Trash.
	Command    []string
	CommandDir string
	Shown      string // how to show Command when it wraps a shell function (nvm)
	Version    string // Node versions: the version, without a leading v
	Regenerate string
	Explain    string
	Note       string

	Measured bool
	Size     size.Size
	Newest   time.Time // newest file inside (node_modules: for local_changes)
	Record   time.Time // node_modules install record, when present

	LastUsed       time.Time
	LastUsedSource string // "atime", "mtime", "commit", "files"

	Ignored, Tracked bool
	GitErr           string
	NestedRepo       bool
	Unreadable       int64 // folders inside that could not be listed

	Safety Safety
	Errors []string

	dev, ino uint64
}

// Action is "command" or "trash".
func (it *Item) Action() string {
	if len(it.Command) > 0 {
		return "command"
	}
	return "trash"
}

// Freeable is what removal is expected to free right away (the measured
// exclusive bytes), or -1 before measuring.
func (it *Item) Freeable() int64 {
	if !it.Measured {
		return -1
	}
	return it.Size.Exclusive()
}

// Options control a scan.
type Options struct {
	Roots  []string // absolute, symlink-free
	Home   string
	Global bool // also list caches, global packages and Node versions
	Env    Env
}

// Result is a scan.
type Result struct {
	Roots    []string
	Projects []*Project
	Items    []*Item
	Warnings []string
	Dirs     int
}

// Find returns the item with this id or path.
func (r *Result) Find(ref string) *Item {
	for _, it := range r.Items {
		if it.ID == ref || pathx.Same(it.Path, ref) {
			return it
		}
	}
	return nil
}

func id(prefix, path string) string {
	h := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	return prefix + "-" + hex.EncodeToString(h[:6])
}

// Scan discovers projects below the roots and, with Global, the machine's
// caches, global packages and Node versions. Sizes come later (Measure).
func Scan(opts Options) (*Result, error) {
	res := &Result{Roots: opts.Roots}
	if !gitx.Available() {
		res.Warnings = append(res.Warnings, "git is not on PATH: items inside repositories cannot be checked and are protected")
	}
	w := newWalker(opts.Home)
	for _, root := range opts.Roots {
		w.walk(root)
	}
	res.Dirs = w.dirs
	if n := len(w.denied); n > 0 {
		slices.Sort(w.denied)
		first := w.denied[0]
		if rel, err := filepath.Rel(opts.Home, first); err == nil && opts.Home != "" && !strings.HasPrefix(rel, "..") {
			first = filepath.Join("~", rel)
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d folder(s) could not be read (permission denied), e.g. %s; projects inside them are not listed", n, first))
	}
	seen := map[string]bool{}
	for _, p := range w.found {
		if !seen[norm(p.Path)] {
			seen[norm(p.Path)] = true
			res.Projects = append(res.Projects, p)
		}
	}
	slices.SortFunc(res.Projects, func(a, b *Project) int { return strings.Compare(a.Path, b.Path) })
	table := Snapshot()
	res.Items = evaluate(res.Projects, table, time.Now())
	if opts.Global {
		g, warns := globals(opts.Env, table)
		pinned(g, res.Projects)
		res.Items = append(res.Items, g...)
		res.Warnings = append(res.Warnings, warns...)
	}
	if !table.CwdKnown {
		res.Warnings = append(res.Warnings, "could not read process working directories: running dev servers cannot be detected, so nothing is preselected")
	}
	return res, nil
}

// pinned notes which projects ask for each installed Node version through
// .nvmrc, .node-version or .tool-versions, so an unused version stands out.
func pinned(items []*Item, projects []*Project) {
	pins := map[*Project]string{}
	for _, p := range projects {
		for _, f := range []string{".nvmrc", ".node-version", ".tool-versions"} {
			if !p.names[f] {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p.Path, f))
			if err != nil || len(b) > 4096 {
				continue
			}
			v := strings.TrimSpace(string(b))
			if f == ".tool-versions" {
				v = ""
				for _, line := range strings.Split(string(b), "\n") {
					if fs := strings.Fields(line); len(fs) >= 2 && fs[0] == "nodejs" {
						v = fs[1]
					}
				}
			}
			if v != "" {
				pins[p] = v
			}
		}
	}
	for _, it := range items {
		if it.Version == "" {
			continue
		}
		var by []string
		for p, v := range pins {
			if matches(v, it.Version) {
				by = append(by, p.Name)
			}
		}
		sort.Strings(by)
		switch {
		case len(by) > 0:
			it.Note = strings.TrimPrefix(it.Note+"; pinned by "+strings.Join(by, ", "), "; ")
		case it.Safety.Level == Review:
			it.Note = strings.TrimPrefix(it.Note+"; no project under the scanned folders pins it", "; ")
		}
	}
}

// evaluate links projects to their workspaces, finds their artifacts and
// gathers git, activity and liveness evidence.
func evaluate(projects []*Project, table *procs.Table, now time.Time) []*Item {
	byPath := map[string]*Project{}
	for _, p := range projects {
		byPath[norm(p.Path)] = p
	}
	for _, p := range projects {
		root := (*Project)(nil)
		for dir := filepath.Dir(p.Path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if a := byPath[norm(dir)]; a != nil && a.workspace {
				root = a
			}
			if p.Repo != "" && !pathx.SameOrWithin(p.Repo, dir) {
				break
			}
		}
		if root != nil {
			p.Group = root.Path
		}
		p.detectPM(root)
	}

	var items []*Item
	for _, p := range projects {
		for _, r := range Rules {
			if !r.When(p) {
				continue
			}
			fi, err := realDir(p.Path, r.Rel, true)
			if err != nil {
				continue
			}
			it := &Item{
				ID: id("art", filepath.Join(p.Path, r.Rel)), Kind: r.Kind, Category: CatProject, Name: r.Rel,
				Path: filepath.Join(p.Path, filepath.FromSlash(r.Rel)), Project: p, Rule: r,
				Regenerate: strings.ReplaceAll(r.Regenerate, "{install}", p.PM.Install()), Explain: r.Explain,
			}
			it.dev, it.ino = size.Identity(fi)
			if r.Kind == "node_modules" {
				it.Record = installRecord(it.Path)
				if len(p.Lockfiles) == 0 {
					it.Explain = "Installed dependencies. There is no lockfile to recreate them from exactly."
				}
			}
			items = append(items, it)
		}
	}
	gitEvidence(items)
	activity(projects, now)
	for _, p := range projects {
		p.CwdKnown = table.CwdKnown
		p.Live = liveIn(table, p)
	}
	for _, it := range items {
		it.classify(now)
	}
	return items
}

// gitEvidence asks git, once per repository, which candidates it ignores
// and which hold tracked files.
func gitEvidence(items []*Item) {
	byRepo := map[string][]*Item{}
	for _, it := range items {
		if r := it.Project.Repo; r != "" {
			byRepo[r] = append(byRepo[r], it)
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for repo, its := range byRepo {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			checkRepo(repo, its)
		}()
	}
	wg.Wait()
}

func checkRepo(repo string, its []*Item) {
	rels := make([]string, len(its))
	var in strings.Builder
	for i, it := range its {
		rel, err := filepath.Rel(repo, it.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			it.GitErr = "outside its repository"
			continue
		}
		rels[i] = filepath.ToSlash(rel)
		in.WriteString(rels[i] + "/\x00")
	}
	// The project folders too: a project git ignores is not source.
	projects := map[*Project]string{}
	for _, it := range its {
		p := it.Project
		if _, seen := projects[p]; seen || it.GitErr != "" {
			continue
		}
		rel, err := filepath.Rel(repo, p.Path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			projects[p] = ""
			continue
		}
		projects[p] = filepath.ToSlash(rel)
		in.WriteString(projects[p] + "/\x00")
	}
	if !gitx.Available() {
		for _, it := range its {
			it.GitErr = "git is not installed"
		}
		return
	}
	out, _, err := gitx.ReadIn(repo, in.String(), "check-ignore", "--stdin", "-z")
	if err != nil {
		for _, it := range its {
			it.GitErr = err.Error()
		}
		return
	}
	ignored := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			ignored[strings.TrimSuffix(p, "/")] = true
		}
	}
	for p, rel := range projects {
		p.GitIgnored = rel != "" && ignored[rel]
	}
	for i := 0; i < len(its); i += 200 {
		chunk := its[i:min(i+200, len(its))]
		args := []string{"ls-files", "-z", "--"}
		for j, it := range chunk {
			if rels[i+j] != "" && it.GitErr == "" {
				args = append(args, rels[i+j])
			}
		}
		out, err := gitx.Read(repo, args...)
		for j, it := range chunk {
			rel := rels[i+j]
			if rel == "" || it.GitErr != "" {
				continue
			}
			if err != nil {
				it.GitErr = err.Error()
				continue
			}
			it.Ignored = ignored[rel]
			for _, f := range strings.Split(string(out), "\x00") {
				if f == rel || strings.HasPrefix(f, rel+"/") {
					it.Tracked = true
					break
				}
			}
		}
	}
}

// activity: the newer of the last commit touching the project and the
// newest source file seen while walking (artifacts excluded). Workspace
// members share their workspace's activity, since a dev server at the root
// uses every member's node_modules.
func activity(projects []*Project, now time.Time) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, p := range projects {
		if p.Repo == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			out, err := gitx.Read(p.Path, "log", "-1", "--format=%ct", "--", ".")
			if sec, perr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil && perr == nil {
				p.LastActivity, p.ActivitySource = time.Unix(sec, 0), "commit"
			}
		}()
	}
	wg.Wait()
	byPath := map[string]*Project{}
	for _, p := range projects {
		if p.newest.After(p.LastActivity) && !p.newest.After(now.Add(time.Hour)) {
			p.LastActivity, p.ActivitySource = p.newest, "files"
		}
		byPath[norm(p.Path)] = p
	}
	for _, p := range projects {
		if g := byPath[norm(p.Group)]; g != nil && g != p && g.LastActivity.After(p.LastActivity) {
			p.LastActivity, p.ActivitySource = g.LastActivity, g.ActivitySource
		}
	}
	for _, p := range projects {
		if g := byPath[norm(p.Group)]; g != nil && g != p && p.LastActivity.After(g.LastActivity) {
			g.LastActivity, g.ActivitySource = p.LastActivity, p.ActivitySource
		}
	}
}

// nodeProcs are executables that read a project's node_modules or caches
// while they run. A shell or editor sitting in the folder does not count.
var nodeProcs = []string{"node", "bun", "deno", "npm", "npx", "pnpm", "yarn", "next-server", "next-router-worker",
	"turbo", "nx", "vite", "esbuild", "tsc", "tsserver", "electron", "Electron", "expo", "metro", "wrangler", "workerd",
	"vitest", "jest", "playwright", "storybook", "astro", "nuxi", "nuxt", "ng", "remix", "react-router", "rspack", "webpack", "parcel"}

// liveIn: Node processes working in the project, or in its workspace root
// (a root `pnpm dev` uses every member's node_modules).
func liveIn(t *procs.Table, p *Project) []procs.Proc {
	var out []procs.Proc
	for _, pr := range t.Procs {
		if pr.Cwd == "" || !slices.ContainsFunc(nodeProcs, func(n string) bool { return strings.EqualFold(n, pr.Command) }) {
			continue
		}
		if pathx.SameOrWithin(p.Path, pr.Cwd) || (pathx.SameOrWithin(p.Group, pr.Cwd) && pathx.SameOrWithin(pr.Cwd, p.Path)) {
			out = append(out, pr)
		}
	}
	return out
}

// installRecord is the mtime of the file the package manager wrote at the
// end of the last install.
func installRecord(nm string) time.Time {
	for _, r := range installRecords {
		if fi, err := os.Lstat(filepath.Join(nm, r)); err == nil {
			return fi.ModTime()
		}
	}
	return time.Time{}
}

// classify turns evidence into a level. It runs again after measuring.
func (it *Item) classify(now time.Time) {
	partial := func(s *Safety) {
		if it.Unreadable > 0 {
			s.add(ReasonPartialRead, Review, fmt.Sprintf("%d folder(s) inside could not be read, so its size and last activity may be incomplete", it.Unreadable))
		}
	}
	if it.Category != CatProject {
		partial(&it.Safety)
		it.fingerprint()
		return
	}
	s := Safety{Level: Safe}
	p, r := it.Project, it.Rule
	switch {
	case r.Level == Protected:
		s.add(ReasonCatalog, Protected, r.Explain)
	case r.Level == Review && (r.Generic || r.Kind == "nuxt-output" || r.Kind == "vercel-output") && r.Kind != "coverage":
		s.add(ReasonDeployable, Review, "build output that may be what you deploy or publish")
	case r.Level == Review:
		s.add(ReasonCatalogReview, Review, r.Explain)
	}
	if r.Kind == "next" && standaloneNext(p) {
		s.add(ReasonDeployable, Review, "next.config uses output: 'standalone'; .next holds the deployable server")
	}
	switch {
	case p.Repo == "":
		if r.Generic {
			s.add(ReasonNoGit, Protected, "not in a git repository, and "+r.Rel+" is a common name for anything")
		}
	case it.GitErr != "":
		s.add(ReasonGitError, Protected, "git could not check it: "+it.GitErr)
	case it.Tracked:
		s.add(ReasonTracked, Protected, "git tracks files inside "+r.Rel)
	case !it.Ignored:
		s.add(ReasonNotIgnored, Protected, "git does not ignore "+r.Rel+", so it may be source")
	}
	if p.GitIgnored {
		s.add(ReasonIgnoredProject, Review, "git ignores the project folder itself, so it is not source: it may be a deployment bundle or local data")
	}
	if it.NestedRepo {
		s.add(ReasonNestedRepo, Protected, "another repository lies inside")
	}
	if p.Worktree {
		s.add(ReasonWorktree, Review, "in a linked git worktree that an agent or app may still be using (fell manages the worktree itself)")
	}
	if len(p.Live) > 0 {
		var names []string
		for _, pr := range p.Live {
			names = append(names, fmt.Sprintf("%s (pid %d)", pr.Command, pr.PID))
		}
		s.add(ReasonInUse, Live, "in use by "+strings.Join(names, ", "))
	} else if !p.CwdKnown {
		s.add(ReasonLivenessUnknown, Review, "running processes could not be checked")
	}
	switch {
	case p.LastActivity.IsZero():
		s.add(ReasonUnknownAge, Review, "last activity unknown")
	case now.Sub(p.LastActivity) < 7*24*time.Hour:
		s.add(ReasonRecent, Review, "project changed in the last 7 days")
	}
	if r.Kind == "node_modules" {
		pms := map[PM]bool{}
		for _, n := range p.Lockfiles {
			for _, l := range lockfiles {
				if l.name == n {
					pms[l.pm] = true
				}
			}
		}
		switch {
		case len(p.Lockfiles) == 0:
			s.add(ReasonNoLockfile, Review, "no lockfile for this project: a reinstall may resolve newer versions, or fail if the folder was copied from elsewhere")
		case len(pms) > 1 && p.PMSource != "packageManager":
			s.add(ReasonAmbiguousPM, Review, "lockfiles of several package managers: "+strings.Join(p.Lockfiles, ", "))
		}
		if it.Measured && !it.Record.IsZero() && it.Newest.After(it.Record.Add(2*time.Minute)) {
			s.add(ReasonLocalChanges, Review, "files changed after the last install ("+it.Newest.Format("2006-01-02")+"); patches would be lost")
		}
	}
	partial(&s)
	it.Safety = s
	it.fingerprint()
}

// fingerprint hashes the safety-relevant state. Sizes and times are left
// out so a build writing into node_modules does not invalidate a plan;
// a changed level or reason does.
func (it *Item) fingerprint() {
	var codes []string
	for _, r := range it.Safety.Reasons {
		codes = append(codes, r.Code)
	}
	sort.Strings(codes)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00%v\x00%v\x00%s\x00%s\x00%s", it.Kind, strings.ToLower(it.Path), it.dev, it.ino,
		it.Ignored, it.Tracked, it.Safety.Level, strings.Join(codes, ","), strings.Join(it.Command, "\x01"))
	it.Fingerprint = "fp-" + hex.EncodeToString(h.Sum(nil)[:8])
}

// skipNewest leaves out of the local_changes check what tools write into
// node_modules after an install by design: hidden top-level entries (the
// install records, .cache, .vite, .prisma, .bin, workspace state) and the
// bookkeeping files directly in pnpm's and Bun's stores. Package contents,
// including those inside .pnpm and .bun, still count.
func skipNewest(rel string, dir bool) bool {
	first, rest, nested := strings.Cut(rel, "/")
	if !nested {
		return strings.HasPrefix(first, ".") && first != ".pnpm" && first != ".bun"
	}
	return (first == ".pnpm" || first == ".bun") && !dir && !strings.Contains(rest, "/")
}

// Measurement is a finished size measurement, applied with Item.Apply on
// the goroutine that owns the result.
type Measurement struct {
	ID     string
	Result size.Result
}

// Measure sizes one item.
func Measure(it *Item) Measurement {
	opts := size.Options{Workers: 8}
	if it.Kind == "node_modules" {
		opts.SkipNewest = skipNewest
	}
	return Measurement{ID: it.ID, Result: size.Measure(it.Path, opts)}
}

// Apply records a measurement and re-checks the item.
func (it *Item) Apply(m Measurement) {
	it.Measured, it.Size, it.Newest = true, m.Result.Size, m.Result.Newest
	it.Unreadable = m.Result.Unreadable
	it.NestedRepo = m.Result.Repo && it.Category == CatProject
	it.classify(time.Now())
}

// MeasureInto measures every item, a few at a time, and sends each result.
// It closes ch when done.
func (r *Result) MeasureInto(ch chan<- Measurement) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, it := range r.Items {
		if it.Measured || it.Path == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			ch <- Measure(it)
		}()
	}
	wg.Wait()
	close(ch)
}

// MeasureAll measures everything in place.
func (r *Result) MeasureAll() {
	ch := make(chan Measurement)
	go r.MeasureInto(ch)
	for m := range ch {
		if it := r.Find(m.ID); it != nil {
			it.Apply(m)
		}
	}
}
