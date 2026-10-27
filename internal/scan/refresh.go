package scan

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/pathx"
	"github.com/Matthew-Eucaristo/nodekill/internal/procs"
)

var (
	procsMu   sync.Mutex
	procsAt   time.Time
	procsLast *procs.Table
)

// freshProcs is a process snapshot at most a few seconds old, so a batch
// of actions does not run lsof once per item.
func freshProcs() *procs.Table {
	procsMu.Lock()
	defer procsMu.Unlock()
	if procsLast == nil || time.Since(procsAt) > 3*time.Second {
		procsLast, procsAt = Snapshot(), time.Now()
	}
	return procsLast
}

// Refresh re-reads one item from disk, measures it and classifies it again,
// so an action decides on current evidence. It returns an error when the
// item no longer exists where the scan saw it.
func Refresh(old *Item, opts Options) (*Item, error) {
	var fresh *Item
	if old.Category == CatProject {
		fresh = refreshProject(old)
	} else {
		items, _ := globals(opts.Env, freshProcs())
		for _, it := range items {
			if it.ID == old.ID {
				fresh = it
			}
		}
	}
	if fresh == nil {
		return nil, errors.New("not found any more")
	}
	if fresh.Path != "" {
		fresh.Apply(Measure(fresh))
	}
	return fresh, nil
}

func refreshProject(old *Item) *Item {
	load := func(dir string) *Project {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		names := map[string]bool{}
		for _, e := range entries {
			names[e.Name()] = true
		}
		if !names["package.json"] {
			return nil
		}
		p := newProject(dir, names)
		p.Repo, p.Worktree = findRepo(dir)
		return p
	}
	p := load(old.Project.Path)
	if p == nil {
		return nil
	}
	p.newest = old.Project.newest
	projects := []*Project{p}
	if g := old.Project.Group; !pathx.Same(g, p.Path) {
		if gp := load(g); gp != nil {
			gp.newest = old.Project.newest
			projects = append(projects, gp)
		}
	}
	for _, it := range evaluate(projects, freshProcs(), time.Now()) {
		if it.Kind == old.Kind && pathx.Same(it.Path, old.Path) {
			return it
		}
	}
	return nil
}

// findRepo walks up to the directory holding .git.
func findRepo(dir string) (string, bool) {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d, linkedWorktree(filepath.Join(d, ".git"))
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}
