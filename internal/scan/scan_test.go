package scan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

const ignoreAll = "node_modules/\n.next/\n.nuxt/\n.output/\n.svelte-kit/\n.turbo/\n.nx/\ndist/\nbuild/\nout/\ncoverage/\n.wrangler/\n.expo/\n.angular/\n.tanstack/\n.parcel-cache/\nstorybook-static/\n.yarn/unplugged/\n"

func pkg(deps string) string {
	return `{"name":"app","dependencies":{` + deps + `}}`
}

// scanFix scans root with the global part off and sizes measured.
func scanFix(t *testing.T, root string) *Result {
	t.Helper()
	res, err := Scan(Options{Roots: []string{root}, Home: t.TempDir(), Env: DefaultEnv()})
	if err != nil {
		t.Fatal(err)
	}
	res.MeasureAll()
	return res
}

func item(t *testing.T, res *Result, path string) *Item {
	t.Helper()
	for _, it := range res.Items {
		if it.Path == path {
			return it
		}
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.Path)
	}
	t.Fatalf("no item at %s; have %v", path, got)
	return nil
}

func noItem(t *testing.T, res *Result, path string) {
	t.Helper()
	for _, it := range res.Items {
		if it.Path == path {
			t.Fatalf("unexpected item at %s (%s)", path, it.Kind)
		}
	}
}

func codes(it *Item) []string {
	var c []string
	for _, r := range it.Safety.Reasons {
		c = append(c, r.Code)
	}
	return c
}

func want(t *testing.T, it *Item, level Level, code ...string) {
	t.Helper()
	if it.Safety.Level != level {
		t.Errorf("%s: level %s, want %s (reasons %v)", it.Path, it.Safety.Level, level, codes(it))
	}
	for _, c := range code {
		if !it.Safety.Has(c) {
			t.Errorf("%s: missing reason %s (have %v)", it.Path, c, codes(it))
		}
	}
}

func TestIdleNextProjectIsSafe(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "shop")
	testfix.Write(t, app, map[string]string{
		"package.json": pkg(`"next":"16.0.0"`), "package-lock.json": "{}", "next.config.mjs": "export default {}",
		".gitignore": ignoreAll, "app/page.tsx": "x",
		"node_modules/next/package.json": "{}", "node_modules/.package-lock.json": "{}",
		".next/cache/turbopack/x": "cache", ".next/dev/cache/turbopack/y": "cache",
	})
	testfix.Repo(t, app, testfix.Old)
	res := scanFix(t, root)
	nm := item(t, res, filepath.Join(app, "node_modules"))
	want(t, nm, Safe)
	if nm.Project.PM != NPM || nm.Regenerate != "npm ci" || nm.Project.Framework() != "Next.js" {
		t.Errorf("pm %s regen %q framework %s", nm.Project.PM, nm.Regenerate, nm.Project.Framework())
	}
	want(t, item(t, res, filepath.Join(app, ".next")), Safe)
	if !nm.Measured || nm.Size.Allocated <= 0 || nm.Size.Files != 2 {
		t.Errorf("size %+v", nm.Size)
	}
	if nm.Fingerprint == "" || nm.ID == "" {
		t.Error("missing id or fingerprint")
	}
	// The static-export rule needs output: 'export'; `export default` is not it.
	noItem(t, res, filepath.Join(app, "out"))
}

func TestGitEvidenceProtects(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{
		"package.json": pkg(`"vite":"7"`), "package-lock.json": "{}", "vite.config.ts": "x",
		".gitignore": "node_modules/\n", "node_modules/a/index.js": "x", "dist/index.html": "x",
		"coverage/keep.txt": "x",
	})
	testfix.Repo(t, app, testfix.Old)
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "node_modules")), Safe)
	want(t, item(t, res, filepath.Join(app, "dist")), Protected, ReasonTracked)

	// Ignored but force-added: tracked files inside an ignored directory.
	testfix.Write(t, app, map[string]string{".gitignore": "node_modules/\ndist/\n"})
	testfix.Git(t, app, testfix.Old, "add", "-f", ".gitignore")
	testfix.Git(t, app, testfix.Old, "commit", "-q", "-m", "ignore dist")
	testfix.Age(t, app, testfix.Old)
	res = scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "dist")), Protected, ReasonTracked)

	// Not ignored and untracked: may be source.
	other := filepath.Join(root, "other")
	testfix.Write(t, other, map[string]string{"package.json": pkg(""), "package-lock.json": "{}", "README.md": "x"})
	testfix.Repo(t, other, testfix.Old)
	testfix.Write(t, other, map[string]string{"node_modules/x/index.js": "x"})
	testfix.Age(t, other, testfix.Old)
	res = scanFix(t, root)
	want(t, item(t, res, filepath.Join(other, "node_modules")), Protected, ReasonNotIgnored)
}

func TestGenericOutputsNeedGitAndMarkers(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	plain := filepath.Join(root, "plain")
	testfix.Write(t, plain, map[string]string{"package.json": pkg(`"vite":"7"`), "package-lock.json": "{}", "dist/a.js": "x", "node_modules/v/a.js": "x"})
	testfix.Age(t, plain, testfix.Old)
	noMarker := filepath.Join(root, "nomarker")
	testfix.Write(t, noMarker, map[string]string{"package.json": pkg(""), "package-lock.json": "{}", "build/a": "x", ".gitignore": ignoreAll})
	testfix.Repo(t, noMarker, testfix.Old)
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(plain, "dist")), Protected, ReasonNoGit)
	want(t, item(t, res, filepath.Join(plain, "node_modules")), Safe) // outside git, node_modules stays eligible
	noItem(t, res, filepath.Join(noMarker, "build"))                  // no builder that writes build/
}

func TestRemotionOutIsNeverRemoved(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "video")
	testfix.Write(t, app, map[string]string{"package.json": pkg(`"remotion":"4", "@remotion/cli":"4"`), "package-lock.json": "{}", ".gitignore": ignoreAll, "out/final.mp4": "x"})
	testfix.Repo(t, app, testfix.Old)
	res := scanFix(t, root)
	it := item(t, res, filepath.Join(app, "out"))
	want(t, it, Protected, ReasonCatalog)
	if it.Kind != "remotion-out" {
		t.Errorf("kind %s", it.Kind)
	}
}

func TestPackageManagerDetection(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	testfix.Write(t, root, map[string]string{
		"mono/package.json": `{"name":"mono","private":true}`, "mono/pnpm-workspace.yaml": "packages: ['packages/*']\n", "mono/pnpm-lock.yaml": "x",
		"mono/packages/web/package.json": pkg(`"@sveltejs/kit":"2"`), "mono/packages/web/node_modules/.modules.yaml": "{}", "mono/packages/web/.svelte-kit/a": "x",
		"mono/.gitignore": ignoreAll, "mono/node_modules/.pnpm/x": "x",
		"classic/package.json": pkg(""), "classic/yarn.lock": "# yarn lockfile v1\n", "classic/node_modules/a": "x",
		"berry/package.json": pkg(""), "berry/yarn.lock": "__metadata:\n  version: 8\n", "berry/node_modules/a": "x",
		"bunapp/package.json": pkg(""), "bunapp/bun.lock": "{}", "bunapp/node_modules/a": "x",
		"field/package.json": `{"packageManager":"yarn@1.22.22"}`, "field/package-lock.json": "{}", "field/node_modules/a": "x",
		"both/package.json": pkg(""), "both/package-lock.json": "{}", "both/yarn.lock": "# v1\n", "both/node_modules/a": "x",
		"none/package.json": pkg(""), "none/node_modules/a": "x",
	})
	testfix.Repo(t, filepath.Join(root, "mono"), testfix.Old)
	testfix.Age(t, root, testfix.Old)
	res := scanFix(t, root)
	web := item(t, res, filepath.Join(root, "mono/packages/web/node_modules"))
	if web.Project.PM != PNPM || web.Project.Group != filepath.Join(root, "mono") {
		t.Errorf("workspace member: pm %s group %s", web.Project.PM, web.Project.Group)
	}
	want(t, item(t, res, filepath.Join(root, "mono/packages/web/.svelte-kit")), Safe)
	for dir, pm := range map[string]PM{"classic": Yarn, "berry": YarnBerry, "bunapp": Bun, "field": Yarn} {
		if got := item(t, res, filepath.Join(root, dir, "node_modules")).Project.PM; got != pm {
			t.Errorf("%s: pm %s, want %s", dir, got, pm)
		}
	}
	want(t, item(t, res, filepath.Join(root, "both/node_modules")), Review, ReasonAmbiguousPM)
	want(t, item(t, res, filepath.Join(root, "none/node_modules")), Review, ReasonNoLockfile)
}

func TestLocalChangesAndRecentActivity(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{"package.json": pkg(""), "package-lock.json": "{}", ".gitignore": ignoreAll,
		"node_modules/.package-lock.json": "{}", "node_modules/lib/index.js": "x", "node_modules/.cache/c": "x"})
	testfix.Repo(t, app, testfix.Old)
	// A cache written after the install is expected; it does not count.
	os.Chtimes(filepath.Join(app, "node_modules/.cache/c"), time.Now(), time.Now())
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "node_modules")), Safe)
	// A package file edited after the install does.
	os.Chtimes(filepath.Join(app, "node_modules/lib/index.js"), time.Now(), time.Now())
	res = scanFix(t, root)
	want(t, item(t, res, filepath.Join(app, "node_modules")), Review, ReasonLocalChanges)

	// A project committed to today is recent.
	fresh := filepath.Join(root, "fresh")
	testfix.Write(t, fresh, map[string]string{"package.json": pkg(""), "package-lock.json": "{}", ".gitignore": ignoreAll, "node_modules/a": "x"})
	testfix.Repo(t, fresh, time.Now())
	res = scanFix(t, root)
	want(t, item(t, res, filepath.Join(fresh, "node_modules")), Review, ReasonRecent)
}

func TestWalkBoundaries(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{"package.json": pkg(`"wrangler":"4"`), "package-lock.json": "{}", ".gitignore": ignoreAll,
		"node_modules/dep/package.json": "{}", "node_modules/dep/node_modules/x/a": "x",
		".wrangler/state/v3/d1/db.sqlite": "data", ".wrangler/tmp/a": "x",
		"Thing.app/Contents/package.json": "{}", "Thing.app/Contents/node_modules/a": "x"})
	testfix.Repo(t, app, testfix.Old)
	elsewhere := filepath.Join(root, "elsewhere")
	testfix.Write(t, elsewhere, map[string]string{"node_modules/a": "x"})
	testfix.Write(t, filepath.Join(root, "linked"), map[string]string{"package.json": pkg("")})
	os.Symlink(filepath.Join(elsewhere, "node_modules"), filepath.Join(root, "linked", "node_modules"))
	res := scanFix(t, root)
	if len(res.Projects) != 2 {
		var got []string
		for _, p := range res.Projects {
			got = append(got, p.Path)
		}
		t.Fatalf("projects %v: dependencies and app bundles must not count", got)
	}
	noItem(t, res, filepath.Join(app, "node_modules/dep/node_modules"))
	noItem(t, res, filepath.Join(app, ".wrangler/state"))
	noItem(t, res, filepath.Join(root, "linked/node_modules")) // symlinks are never items
	want(t, item(t, res, filepath.Join(app, ".wrangler/tmp")), Safe)
}

func TestNextVariants(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	testfix.Write(t, root, map[string]string{
		"sa/package.json": pkg(`"next":"16"`), "sa/package-lock.json": "{}", "sa/next.config.js": "module.exports = { output: 'standalone' }", "sa/.gitignore": ignoreAll, "sa/.next/standalone/server.js": "x",
		"ex/package.json": pkg(`"next":"16"`), "ex/package-lock.json": "{}", "ex/next.config.ts": "export default { output: \"export\" }", "ex/.gitignore": ignoreAll, "ex/out/index.html": "x",
	})
	testfix.Repo(t, filepath.Join(root, "sa"), testfix.Old)
	testfix.Repo(t, filepath.Join(root, "ex"), testfix.Old)
	res := scanFix(t, root)
	want(t, item(t, res, filepath.Join(root, "sa/.next")), Review, ReasonDeployable)
	want(t, item(t, res, filepath.Join(root, "ex/out")), Review, ReasonDeployable)
}

func TestNestedRepoIsProtected(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{"package.json": pkg(""), "package-lock.json": "{}", ".gitignore": ignoreAll, "node_modules/gitdep/.git/HEAD": "ref"})
	testfix.Repo(t, app, testfix.Old)
	want(t, item(t, scanFix(t, root), filepath.Join(app, "node_modules")), Protected, ReasonNestedRepo)
}

func TestRefreshKeepsFingerprint(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	app := filepath.Join(root, "app")
	testfix.Write(t, app, map[string]string{"package.json": pkg(`"turbo":"2"`), "turbo.json": "{}", "package-lock.json": "{}", ".gitignore": ignoreAll, "node_modules/a": "x", ".turbo/cache/x": "x"})
	testfix.Repo(t, app, testfix.Old)
	res := scanFix(t, root)
	it := item(t, res, filepath.Join(app, ".turbo", "cache"))
	fresh, err := Refresh(it, Options{Env: DefaultEnv()})
	if err != nil || fresh.Fingerprint != it.Fingerprint {
		t.Fatalf("refresh: %v, %s vs %s", err, fresh.Fingerprint, it.Fingerprint)
	}
	os.WriteFile(filepath.Join(app, ".gitignore"), []byte("node_modules/\n"), 0o644)
	fresh, _ = Refresh(it, Options{Env: DefaultEnv()})
	if fresh.Fingerprint == it.Fingerprint || !slices.Contains(codes(fresh), ReasonNotIgnored) {
		t.Fatalf("fingerprint should change when git stops ignoring it: %v", codes(fresh))
	}
}

func TestWorkspaceMembership(t *testing.T) {
	for _, c := range []struct {
		yaml  string
		globs []string
		ok    bool
	}{
		{"packages:\n  - apps/*\n  - 'packages/**' # libs\n  - \"!**/test/**\"\nallowBuilds:\n  esbuild: true\n", []string{"apps/*", "packages/**", "!**/test/**"}, true},
		{"packages:\n- apps/*\n", []string{"apps/*"}, true},
		{"packages: ['apps/*', \"libs/*\"]\n", []string{"apps/*", "libs/*"}, true},
		{"packages: []\n", nil, true},
		{"onlyBuiltDependencies:\n  - esbuild\n", nil, false}, // no list: pnpm includes every subfolder
		{"packages:\n  key: value\n", nil, false},
	} {
		globs, ok := pnpmPackages(c.yaml)
		if ok != c.ok || !slices.Equal(globs, c.globs) {
			t.Errorf("%q: got %v %v, want %v %v", c.yaml, globs, ok, c.globs, c.ok)
		}
	}
	w := &Project{Path: "/mono", globs: []string{"./apps/*", "packages/**", "!**/test/**"}, globsKnown: true}
	for rel, in := range map[string]bool{"apps/web": true, "apps/web/sub": false, "packages/a/b": true, "packages/a/test/x": false,
		".data/prod/api": false, "docs": false} {
		if got := w.includes(&Project{Path: filepath.Join("/mono", rel)}); got != in {
			t.Errorf("includes %s = %v, want %v", rel, got, in)
		}
	}
	if !(&Project{Path: "/mono"}).includes(&Project{Path: "/mono/anything"}) {
		t.Error("unknown patterns must count every subfolder as a member")
	}
}

// A deployment bundle copied into a gitignored folder of a monorepo is not
// a workspace member: it does not share the root's lockfile, and nodekill
// must not claim it can be recreated from one.
func TestDeployBundleIsNotAWorkspaceMember(t *testing.T) {
	root := testfix.Real(t, t.TempDir())
	mono := filepath.Join(root, "mono")
	testfix.Write(t, mono, map[string]string{
		"package.json": `{"name":"mono","private":true}`, "pnpm-workspace.yaml": "packages:\n  - apps/*\n", "pnpm-lock.yaml": "x",
		".gitignore": ".data\nnode_modules/\n", "node_modules/.pnpm/x": "x",
		"apps/api/package.json": pkg(""), "apps/api/node_modules/.modules.yaml": "{}",
		".data/prod/api/package.json": pkg(""), ".data/prod/api/dist/main.js": "x", ".data/prod/api/node_modules/a/index.js": "x",
		".data/prod/secrets.env": "x", ".data/pg/base": "x",
	})
	testfix.Repo(t, mono, testfix.Old)
	res := scanFix(t, root)

	member := item(t, res, filepath.Join(mono, "apps/api/node_modules"))
	want(t, member, Safe)
	if !slices.Equal(member.Project.Lockfiles, []string{"pnpm-lock.yaml"}) || member.Explain != Rules[0].Explain {
		t.Errorf("member: lockfiles %v, explain %q", member.Project.Lockfiles, member.Explain)
	}

	bundle := item(t, res, filepath.Join(mono, ".data/prod/api/node_modules"))
	want(t, bundle, Review, ReasonNoLockfile, ReasonIgnoredProject)
	if len(bundle.Project.Lockfiles) != 0 || strings.Contains(bundle.Explain, "from the lockfile") {
		t.Errorf("bundle: lockfiles %v, explain %q", bundle.Project.Lockfiles, bundle.Explain)
	}
	if member.Safety.Has(ReasonIgnoredProject) {
		t.Error("a member git does not ignore is not an ignored project")
	}
}
