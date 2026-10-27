package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/pathx"
	"github.com/Matthew-Eucaristo/nodekill/internal/size"
)

// walker finds projects (directories with a package.json) below the roots.
// It never follows symlinks, stays on each root's filesystem, and never
// enters dependency or build directories, other tools' homes, app bundles,
// cloud-only placeholders or the privacy-protected home folders (unless a
// root is inside one).
type walker struct {
	skip    map[string]bool // normalised absolute paths never entered
	private map[string]bool // home folders that trigger macOS privacy prompts
	sem     chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	found   []*Project
	dirs    int
	denied  []string // folders that could not be listed
}

func newWalker(home string) *walker {
	w := &walker{skip: map[string]bool{}, private: map[string]bool{}, sem: make(chan struct{}, max(8, 2*runtime.NumCPU()))}
	var skip []string
	if home != "" {
		for _, rel := range []string{
			"Library", ".Trash", ".npm", ".nvm", ".fnm", ".volta", ".bun", ".deno", ".yarn", ".pnpm-store", ".cache",
			".config", ".local", ".vscode", ".vscode-server", ".cursor", ".windsurf", ".gradle", ".m2", ".cargo",
			".rustup", ".pub-cache", ".android", ".docker", ".orbstack", "go/pkg", ".asdf", ".n", ".node-gyp",
			"AppData", "Applications",
		} {
			skip = append(skip, filepath.Join(home, rel))
		}
		for _, rel := range []string{"Desktop", "Documents", "Downloads", "Pictures", "Movies", "Music", "Public"} {
			w.private[norm(filepath.Join(home, rel))] = true
		}
	}
	skip = append(skip, "/Applications", "/System", "/Library", "/opt/homebrew", "/usr", "/private", "/proc", "/sys", "/dev", "/nix",
		`C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`)
	for _, p := range skip {
		w.skip[norm(p)] = true
	}
	return w
}

func norm(p string) string { return strings.ToLower(filepath.Clean(p)) }

// walk visits root and everything below it.
func (w *walker) walk(root string) {
	fi, err := os.Lstat(root)
	if err != nil {
		return
	}
	dev, _ := size.Identity(fi)
	// A root inside a protected folder was chosen on purpose.
	for p := range w.private {
		if pathx.SameOrWithin(p, root) {
			delete(w.private, p)
		}
	}
	w.wg.Add(1)
	w.visit(root, dev, "", false, nil)
	w.wg.Wait()
}

func (w *walker) visit(dir string, dev uint64, repo string, worktree bool, owner *Project) {
	defer w.wg.Done()
	f, err := os.Open(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			w.mu.Lock()
			w.denied = append(w.denied, dir)
			w.mu.Unlock()
		}
		return
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		w.mu.Lock()
		w.denied = append(w.denied, dir)
		w.mu.Unlock()
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	w.mu.Lock()
	w.dirs++
	w.mu.Unlock()

	if names[".git"] {
		repo, worktree = dir, linkedWorktree(filepath.Join(dir, ".git"))
	}
	if fi, err := os.Lstat(filepath.Join(dir, "package.json")); err == nil && fi.Mode().IsRegular() {
		p := newProject(dir, names)
		p.Repo, p.Worktree = repo, worktree && repo != ""
		w.mu.Lock()
		w.found = append(w.found, p)
		w.mu.Unlock()
		owner = p
	}

	var newest time.Time
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		if e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		if !e.IsDir() {
			if owner != nil && e.Type().IsRegular() {
				if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
			continue
		}
		if noDescend[name] || strings.HasSuffix(name, ".app") || w.skip[norm(path)] || w.private[norm(path)] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if d, _ := size.Identity(info); d != dev && d != 0 {
			continue // another filesystem
		}
		if dataless(info) {
			continue // iCloud or File Provider placeholder: reading would download it
		}
		w.wg.Add(1)
		select {
		case w.sem <- struct{}{}:
			go func() {
				defer func() { <-w.sem }()
				w.visit(path, dev, repo, worktree, owner)
			}()
		default:
			w.visit(path, dev, repo, worktree, owner)
		}
	}
	if owner != nil && !newest.IsZero() {
		w.mu.Lock()
		if newest.After(owner.newest) {
			owner.newest = newest
		}
		w.mu.Unlock()
	}
}

// linkedWorktree: a .git file pointing into <common>/worktrees/<name>.
func linkedWorktree(dotgit string) bool {
	fi, err := os.Lstat(dotgit)
	if err != nil || fi.IsDir() {
		return false
	}
	b, err := os.ReadFile(dotgit)
	if err != nil || len(b) > 4096 {
		return false
	}
	gd, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	return ok && filepath.Base(filepath.Dir(strings.TrimSpace(gd))) == "worktrees"
}

// exists reports whether rel (slash-separated) exists below the project
// without passing through a symlink.
func exists(p *Project, rel string) bool {
	_, err := realDir(p.Path, rel, false)
	return err == nil
}

// realDir walks rel below root with Lstat, refusing symlinks on the way.
// With wantDir the last component must be a directory.
func realDir(root, rel string, wantDir bool) (fs.FileInfo, error) {
	cur := root
	var fi fs.FileInfo
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		var err error
		if fi, err = os.Lstat(cur); err != nil {
			return nil, err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, &fs.PathError{Op: "lstat", Path: cur, Err: errSymlink}
		}
		if (i < len(parts)-1 || wantDir) && !fi.IsDir() {
			return nil, &fs.PathError{Op: "lstat", Path: cur, Err: errNotDir}
		}
	}
	return fi, nil
}
