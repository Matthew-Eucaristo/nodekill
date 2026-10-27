package scan

import "regexp"

// Rule is one rebuildable directory a JS project can hold. A directory is
// a candidate only at its exact position (Rel, below the package.json) and
// only when When proves the project uses the tool that makes it; names alone
// are never enough. Generic names (dist, build, out) are review at best and
// protected outside git.
type Rule struct {
	Kind       string // stable id, used in JSON
	Rel        string // slash-separated, relative to the project
	Level      Level  // Safe or Review before evidence is applied
	Generic    bool   // a common name that could hold anything
	Regenerate string // how it comes back; "{install}" = the project's install command
	Explain    string
	When       func(p *Project) bool
}

func always(*Project) bool { return true }

func dep(names ...string) func(*Project) bool {
	return func(p *Project) bool {
		for _, n := range names {
			if p.has(n) {
				return true
			}
		}
		return false
	}
}

func cfg(names ...string) func(*Project) bool {
	return func(p *Project) bool {
		for _, n := range names {
			if p.config(n) {
				return true
			}
		}
		return false
	}
}

func anyOf(fs ...func(*Project) bool) func(*Project) bool {
	return func(p *Project) bool {
		for _, f := range fs {
			if f(p) {
				return true
			}
		}
		return false
	}
}

func allOf(fs ...func(*Project) bool) func(*Project) bool {
	return func(p *Project) bool {
		for _, f := range fs {
			if !f(p) {
				return false
			}
		}
		return true
	}
}

func not(f func(*Project) bool) func(*Project) bool { return func(p *Project) bool { return !f(p) } }

var (
	remotion = anyOf(dep("remotion", "@remotion/cli"), cfg("remotion.config"))
	native   = dep("react-native", "expo")
)

// Rules is the catalog, in display order. Sources for each entry are listed
// in docs/rules.md.
var Rules = []*Rule{
	{Kind: "node_modules", Rel: "node_modules", Level: Safe, Regenerate: "{install}", When: always,
		Explain: "Installed dependencies. Your package manager recreates them from the lockfile."},
	{Kind: "next", Rel: ".next", Level: Safe, Regenerate: "next dev / next build", When: anyOf(dep("next"), cfg("next.config")),
		Explain: "Next.js build output and caches, including the Turbopack caches in .next/cache and .next/dev/cache, which Next.js does not prune."},
	{Kind: "nuxt", Rel: ".nuxt", Level: Safe, Regenerate: "nuxt dev / nuxt build (nuxi cleanup removes it too)", When: anyOf(dep("nuxt"), cfg("nuxt.config")),
		Explain: "Nuxt's generated build directory."},
	{Kind: "nuxt-output", Rel: ".output", Level: Review, Regenerate: "nuxt build", When: anyOf(dep("nuxt", "nitropack"), cfg("nuxt.config", "nitro.config")),
		Explain: "Nuxt/Nitro production bundle. Rebuildable, but it may be what you deploy."},
	{Kind: "sveltekit", Rel: ".svelte-kit", Level: Safe, Regenerate: "svelte-kit sync / vite dev", When: anyOf(dep("@sveltejs/kit"), cfg("svelte.config")),
		Explain: "SvelteKit's generated types and build output."},
	{Kind: "angular-cache", Rel: ".angular/cache", Level: Safe, Regenerate: "ng build / ng serve (ng cache clean removes it too)", When: cfg("angular.json"),
		Explain: "Angular CLI build cache."},
	{Kind: "turbo", Rel: ".turbo", Level: Safe, Regenerate: "the next turbo run (cache misses once)", When: anyOf(dep("turbo"), cfg("turbo.json")),
		Explain: "Turborepo's local task cache and logs."},
	{Kind: "nx-cache", Rel: ".nx/cache", Level: Safe, Regenerate: "the next nx run (nx reset removes it too)", When: cfg("nx.json"),
		Explain: "Nx's local task cache (older Nx versions)."},
	{Kind: "nx-workspace-data", Rel: ".nx/workspace-data", Level: Safe, Regenerate: "the next nx command (nx reset removes it too)", When: cfg("nx.json"),
		Explain: "Nx project graph and daemon data."},
	{Kind: "parcel-cache", Rel: ".parcel-cache", Level: Safe, Regenerate: "the next parcel build", When: anyOf(dep("parcel"), cfg(".parcelrc")),
		Explain: "Parcel's build cache."},
	{Kind: "expo", Rel: ".expo", Level: Safe, Regenerate: "expo start", When: dep("expo"),
		Explain: "Expo's per-machine dev state (device list, dev-server settings)."},
	{Kind: "docusaurus", Rel: ".docusaurus", Level: Safe, Regenerate: "docusaurus start / build (docusaurus clear removes it too)", When: cfg("docusaurus.config"),
		Explain: "Docusaurus generated files."},
	{Kind: "react-router", Rel: ".react-router", Level: Safe, Regenerate: "react-router typegen / dev", When: anyOf(dep("@react-router/dev"), cfg("react-router.config")),
		Explain: "React Router generated route types."},
	{Kind: "vinxi", Rel: ".vinxi", Level: Safe, Regenerate: "vinxi dev / build", When: dep("vinxi"),
		Explain: "Vinxi build cache (older TanStack Start)."},
	{Kind: "tanstack-tmp", Rel: ".tanstack/tmp", Level: Safe, Regenerate: "the TanStack router generator", When: func(p *Project) bool { return p.hasPrefix("@tanstack/") },
		Explain: "TanStack Router/Start generator scratch files."},
	{Kind: "wrangler-tmp", Rel: ".wrangler/tmp", Level: Safe, Regenerate: "wrangler dev", When: anyOf(dep("wrangler"), cfg("wrangler.toml", "wrangler.json", "wrangler.jsonc")),
		Explain: "Wrangler build scratch. Local D1/KV/R2 data in .wrangler/state is never touched."},
	{Kind: "wrangler-cache", Rel: ".wrangler/cache", Level: Safe, Regenerate: "wrangler dev", When: anyOf(dep("wrangler"), cfg("wrangler.toml", "wrangler.json", "wrangler.jsonc")),
		Explain: "Wrangler cache. Local D1/KV/R2 data in .wrangler/state is never touched."},
	{Kind: "vercel-output", Rel: ".vercel/output", Level: Review, Regenerate: "vercel build", When: anyOf(dep("vercel"), cfg("vercel.json"), func(p *Project) bool { return p.file(".vercel") }),
		Explain: "Prebuilt Vercel output. The project link and pulled env files in .vercel are never touched."},
	{Kind: "gatsby-cache", Rel: ".cache", Level: Safe, Regenerate: "gatsby develop / build (gatsby clean removes it too)", When: cfg("gatsby-config"),
		Explain: "Gatsby's build cache."},
	{Kind: "firebase", Rel: ".firebase", Level: Safe, Regenerate: "the next firebase deploy", When: cfg("firebase.json"),
		Explain: "Firebase Hosting upload cache."},
	{Kind: "yarn-unplugged", Rel: ".yarn/unplugged", Level: Safe, Regenerate: "yarn install", When: cfg(".yarnrc.yml"),
		Explain: "Yarn Plug'n'Play unpacked packages. The committed .yarn/cache is never touched."},
	{Kind: "nyc-output", Rel: ".nyc_output", Level: Safe, Regenerate: "the next nyc run", When: dep("nyc"),
		Explain: "nyc coverage raw data."},
	{Kind: "coverage", Rel: "coverage", Level: Review, Generic: true, Regenerate: "run the tests with coverage", When: anyOf(dep("jest", "vitest", "nyc", "c8"), func(p *Project) bool { return p.hasPrefix("@vitest/coverage") }),
		Explain: "Coverage report. Rebuildable by running the tests."},
	{Kind: "storybook-static", Rel: "storybook-static", Level: Review, Generic: true, Regenerate: "storybook build", When: anyOf(dep("storybook"), cfg(".storybook")),
		Explain: "Static Storybook build. Rebuildable, but it may be what you publish."},
	{Kind: "rn-pods", Rel: "ios/Pods", Level: Review, Regenerate: "pod install (npx pod-install)", When: allOf(native, func(p *Project) bool { return exists(p, "ios/Podfile") }),
		Explain: "CocoaPods dependencies of the iOS app. Some teams commit them; tracked Pods are never touched."},
	{Kind: "rn-ios-build", Rel: "ios/build", Level: Safe, Regenerate: "the next iOS build", When: allOf(native, func(p *Project) bool { return exists(p, "ios/Podfile") }),
		Explain: "Xcode build output of the iOS app."},
	{Kind: "rn-android-build", Rel: "android/app/build", Level: Safe, Regenerate: "the next Android build", When: allOf(native, androidProject),
		Explain: "Gradle build output of the Android app."},
	{Kind: "rn-android-gradle", Rel: "android/.gradle", Level: Safe, Regenerate: "the next Gradle run", When: allOf(native, androidProject),
		Explain: "Gradle's project cache for the Android app."},
	{Kind: "dist", Rel: "dist", Level: Review, Generic: true, Regenerate: "your build script", When: anyOf(dep("vite", "astro", "parcel", "tsup", "electron-builder", "@angular/core", "rollup", "webpack"), cfg("vite.config", "astro.config", "angular.json", "electron-builder")),
		Explain: "Build output. Rebuildable, but it may be what you publish or deploy."},
	{Kind: "build", Rel: "build", Level: Review, Generic: true, Regenerate: "your build script", When: anyOf(dep("react-scripts", "@docusaurus/core", "@react-router/dev", "@remix-run/dev", "@sveltejs/adapter-static", "@sveltejs/adapter-node"), cfg("docusaurus.config", "react-router.config", "remix.config")),
		Explain: "Build output. Rebuildable, but it may be what you publish or deploy."},
	{Kind: "out", Rel: "out", Level: Review, Generic: true, Regenerate: "next build (static export) / electron-forge make", When: allOf(not(remotion), anyOf(nextExport, cfg("forge.config"))),
		Explain: "Static export or packaged app. Rebuildable, but it may be what you publish."},
	{Kind: "remotion-out", Rel: "out", Level: Protected, Generic: true, Regenerate: "re-render the video", When: remotion,
		Explain: "Remotion's render output: your videos, not a cache. nodekill never removes it."},
}

var (
	outputExport     = regexp.MustCompile(`output\s*:\s*["'` + "`" + `]export["'` + "`" + `]`)
	outputStandalone = regexp.MustCompile(`output\s*:\s*["'` + "`" + `]standalone["'` + "`" + `]`)
)

func nextExport(p *Project) bool {
	return (p.has("next") || p.config("next.config")) && outputExport.MatchString(p.configText("next.config"))
}

func androidProject(p *Project) bool {
	return exists(p, "android/build.gradle") || exists(p, "android/build.gradle.kts")
}

// standaloneNext: `output: 'standalone'` puts a deployable server in .next.
func standaloneNext(p *Project) bool {
	return outputStandalone.MatchString(p.configText("next.config"))
}

// noDescend are directory names the project walk never enters: they hold
// no projects of their own, only dependencies, caches and build output.
var noDescend = map[string]bool{
	"node_modules": true, ".git": true, ".next": true, ".nuxt": true, ".output": true, ".svelte-kit": true,
	".turbo": true, ".nx": true, ".parcel-cache": true, ".angular": true, ".expo": true, ".docusaurus": true,
	".react-router": true, ".vinxi": true, ".tanstack": true, ".wrangler": true, ".vercel": true, ".netlify": true,
	".yarn": true, ".cache": true, ".firebase": true, ".pnpm-store": true, "Pods": true, "DerivedData": true,
	".gradle": true, ".nyc_output": true, "storybook-static": true, ".Trash": true, ".svn": true, ".hg": true,
	"bower_components": true, "jspm_packages": true, ".venv": true, "__pycache__": true, "target": true,
}
