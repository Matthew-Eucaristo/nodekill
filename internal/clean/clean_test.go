package clean

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Matthew-Eucaristo/nodekill/internal/scan"
	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

// fakeTrash replaces the trash with a folder in the test's temp dir.
func fakeTrash(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	old := moveFunc
	moveFunc = func(p string) error { return os.Rename(p, filepath.Join(bin, filepath.Base(p))) }
	t.Cleanup(func() { moveFunc = old })
	return bin
}

func setup(t *testing.T, files map[string]string, commit bool) (string, *scan.Result, scan.Options) {
	t.Helper()
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, files)
	if commit {
		testfix.Repo(t, app, testfix.Old)
	} else {
		testfix.Age(t, app, testfix.Old)
	}
	opts := scan.Options{Roots: []string{root}, Home: t.TempDir(), Env: scan.DefaultEnv()}
	res, err := scan.Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	res.MeasureAll()
	return app, res, opts
}

var base = map[string]string{
	"package.json": `{"dependencies":{"next":"16"}}`, "package-lock.json": "{}", ".gitignore": "node_modules/\n.next/\ndist/\n",
	"node_modules/next/index.js": "x", "node_modules/.package-lock.json": "{}", ".next/cache/a": "x", "src/page.tsx": "x",
}

func TestTrashSafeArtifact(t *testing.T) {
	bin := fakeTrash(t)
	app, res, opts := setup(t, base, true)
	it := res.Find(filepath.Join(app, "node_modules"))
	out := Act(it, it.Fingerprint, Options{Mode: Trash, Scan: opts})
	if out.Status != StatusDone || out.Action != "trash" {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Lstat(filepath.Join(app, "node_modules")); !os.IsNotExist(err) {
		t.Error("node_modules still there")
	}
	if _, err := os.Lstat(filepath.Join(bin, "node_modules", "next", "index.js")); err != nil {
		t.Error("not in the trash")
	}
	if _, err := os.Lstat(filepath.Join(app, "src", "page.tsx")); err != nil {
		t.Error("source touched")
	}
	if out.Estimate <= 0 || out.RestoreHint == "" {
		t.Errorf("estimate %d hint %q", out.Estimate, out.RestoreHint)
	}
}

func TestDryRunAndPermanent(t *testing.T) {
	fakeTrash(t)
	app, res, opts := setup(t, base, true)
	it := res.Find(filepath.Join(app, ".next"))
	if out := Act(it, it.Fingerprint, Options{Mode: DryRun, Scan: opts}); out.Status != StatusDryRun {
		t.Fatalf("dry run: %+v", out)
	}
	if _, err := os.Lstat(filepath.Join(app, ".next")); err != nil {
		t.Fatal("dry run removed it")
	}
	if out := Act(it, it.Fingerprint, Options{Mode: Permanent, Scan: opts}); out.Status != StatusDone || out.Action != "delete" {
		t.Fatalf("permanent: %+v", out)
	}
	if _, err := os.Lstat(filepath.Join(app, ".next")); !os.IsNotExist(err) {
		t.Error(".next still there")
	}
}

func TestRefusals(t *testing.T) {
	fakeTrash(t)
	files := map[string]string{}
	for k, v := range base {
		files[k] = v
	}
	files["dist/index.js"] = "x"
	files["src/vite.config.ts"] = "x"
	app, res, opts := setup(t, files, true)

	// Stale: the scan's fingerprint no longer matches.
	nm := res.Find(filepath.Join(app, "node_modules"))
	if out := Act(nm, "fp-0000", Options{Mode: Trash, Scan: opts}); out.Status != StatusRefused {
		t.Errorf("stale plan: %+v", out)
	}
	// Changed since the scan: git stops ignoring it.
	os.WriteFile(filepath.Join(app, ".gitignore"), []byte(".next/\n"), 0o644)
	if out := Act(nm, nm.Fingerprint, Options{Mode: Trash, Scan: opts}); out.Status != StatusRefused {
		t.Errorf("changed item: %+v", out)
	}
	if _, err := os.Lstat(filepath.Join(app, "node_modules")); err != nil {
		t.Fatal("refused item was removed")
	}
}

func TestReviewNeedsConsent(t *testing.T) {
	fakeTrash(t)
	files := map[string]string{"package.json": `{}`, ".gitignore": "node_modules/\n", "node_modules/a": "x"} // no lockfile → review
	app, res, opts := setup(t, files, true)
	it := res.Find(filepath.Join(app, "node_modules"))
	if it.Safety.Level != scan.Review {
		t.Fatalf("level %s", it.Safety.Level)
	}
	if out := Act(it, it.Fingerprint, Options{Mode: Trash, Scan: opts}); out.Status != StatusRefused {
		t.Errorf("review without consent: %+v", out)
	}
	if out := Act(it, it.Fingerprint, Options{Mode: Trash, Review: true, Scan: opts}); out.Status != StatusDone {
		t.Errorf("review with consent: %+v", out)
	}
}

func TestProtectedNeverRemoved(t *testing.T) {
	fakeTrash(t)
	files := map[string]string{"package.json": `{"dependencies":{"remotion":"4"}}`, "package-lock.json": "{}", ".gitignore": "out/\n", "out/video.mp4": "x"}
	app, res, opts := setup(t, files, true)
	it := res.Find(filepath.Join(app, "out"))
	for _, m := range []Mode{Trash, Permanent} {
		if out := Act(it, it.Fingerprint, Options{Mode: m, Review: true, Scan: opts}); out.Status != StatusRefused {
			t.Errorf("%s: %+v", m, out)
		}
	}
	if _, err := os.Lstat(filepath.Join(app, "out", "video.mp4")); err != nil {
		t.Fatal("render output removed")
	}
}

func TestPermanentDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	fakeTrash(t)
	app, res, opts := setup(t, base, true)
	// A workspace link inside node_modules points at source outside it.
	os.MkdirAll(filepath.Join(app, "packages", "lib"), 0o755)
	os.WriteFile(filepath.Join(app, "packages", "lib", "index.js"), []byte("source"), 0o644)
	os.Symlink(filepath.Join(app, "packages", "lib"), filepath.Join(app, "node_modules", "lib"))
	it := res.Find(filepath.Join(app, "node_modules"))
	fresh, _ := scan.Refresh(it, opts)
	if out := Act(fresh, fresh.Fingerprint, Options{Mode: Permanent, Scan: opts}); out.Status != StatusDone {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Lstat(filepath.Join(app, "packages", "lib", "index.js")); err != nil {
		t.Fatal("followed a symlink out of node_modules")
	}
}

func TestCheckRefusesBroadAndForeign(t *testing.T) {
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/", home, "relative/path", filepath.Dir(home)} {
		if err := checkPath(p); err == nil {
			t.Errorf("checkPath(%q) allowed", p)
		}
	}
	it := &scan.Item{Command: []string{"/bin/rm", "-rf", "/"}}
	if err := Check(it); err == nil {
		t.Error("ran a non-Node tool")
	}
	if err := Check(&scan.Item{Command: []string{"npm", "cache", "clean"}}); err == nil {
		t.Error("ran a relative executable")
	}
}
