package clean

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Matthew-Eucaristo/nodekill/internal/pathx"
	"github.com/Matthew-Eucaristo/nodekill/internal/scan"
)

// These checks run on freshly read evidence immediately before every
// removal, independently of how the item was chosen, so a stale or
// tampered plan cannot widen what is removed.

// officialTools are the only executables nodekill runs to clean up.
var officialTools = []string{"npm", "pnpm", "yarn", "bun", "deno", "corepack", "fnm", "mise", "n", "nvm", "asdf", "nodenv", "brew", "npx", "node"}

// Check validates a freshly scanned item right before acting on it.
func Check(it *scan.Item) error {
	if len(it.Command) > 0 {
		return checkCommand(it)
	}
	if err := checkPath(it.Path); err != nil {
		return err
	}
	if it.Category == scan.CatProject {
		return checkArtifact(it)
	}
	return nil
}

// checkPath refuses relative, broad or symlinked targets.
func checkPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("refusing %q: not a clean absolute path", path)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink %s", path)
	}
	home, _ := os.UserHomeDir()
	parent := filepath.Dir(path)
	if parent == path || filepath.Dir(parent) == parent || (home != "" && pathx.SameOrWithin(path, home)) {
		return fmt.Errorf("refusing broad path %s", path)
	}
	return nil
}

// checkArtifact: the directory sits at the catalog position below a real
// project, with no symlink on the way, and holds no repository.
func checkArtifact(it *scan.Item) error {
	p, r := it.Project, it.Rule
	if p == nil || r == nil {
		return errors.New("not a project artifact")
	}
	want := filepath.Join(p.Path, filepath.FromSlash(r.Rel))
	if !pathx.Same(want, it.Path) || !pathx.Within(p.Path, it.Path) {
		return fmt.Errorf("%s is not %s of %s", it.Path, r.Rel, p.Path)
	}
	if fi, err := os.Lstat(filepath.Join(p.Path, "package.json")); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no package.json any more", p.Path)
	}
	cur := p.Path
	for _, part := range strings.Split(r.Rel, "/") {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return fmt.Errorf("refusing: %s is not a real directory", cur)
		}
	}
	if it.NestedRepo {
		return fmt.Errorf("%s contains a repository; remove it by hand", it.Path)
	}
	return nil
}

// checkCommand: the executable is one of the official tools, resolved to an
// absolute path, and the command came from nodekill's own catalog.
func checkCommand(it *scan.Item) error {
	exe := it.Command[0]
	if !filepath.IsAbs(exe) {
		return fmt.Errorf("refusing %q: not an absolute executable path", exe)
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(exe)), ".exe")
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".cmd"), ".ps1")
	if !slices.Contains(officialTools, base) && base != "bash" && base != "sh" {
		return fmt.Errorf("refusing to run %s: not a known Node tool", exe)
	}
	if fi, err := os.Stat(exe); err != nil || fi.IsDir() {
		return fmt.Errorf("%s is not runnable", exe)
	}
	return nil
}
