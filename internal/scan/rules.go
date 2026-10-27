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

// tool: a dependency here or in the workspace root (hoisted test runners
// and build tools).
func tool(names ...string) func(*Project) bool {
	return func(p *Project) bool {
		for _, n := range names {
			if p.usesTool(n) {
				return true
			}
		}
		return false
	}
}

// at: a file or folder exists at rel below the project.
func at(rels ...string) func(*Project) bool {
	return func(p *Project) bool {
		for _, r := range rels {
			if exists(p, r) {
				return true
			}
		}
		return false
	}
}

var (
	remotion   = anyOf(dep("remotion", "@remotion/cli"), cfg("remotion.config"))
	native     = dep("react-native", "expo")
	capacitor  = anyOf(dep("@capacitor/core", "@capacitor/cli"), cfg("capacitor.config"))
	turbo      = anyOf(tool("turbo"), cfg("turbo.json", "turbo.jsonc"))
	wrangler   = anyOf(dep("wrangler"), cfg("wrangler.toml", "wrangler.json", "wrangler.jsonc"))
	playwright = anyOf(tool("@playwright/test", "playwright"), cfg("playwright.config"))
	cypress    = anyOf(tool("cypress"), cfg("cypress.config"))
	vitepress  = tool("vitepress")
	hardhat    = cfg("hardhat.config")
	foundry    = cfg("foundry.toml")
)

// Rules is the catalog, in display order. Sources for each entry are listed
// in docs/rules.md.
var Rules = []*Rule{
	{Kind: "node_modules", Rel: "node_modules", Level: Safe, Regenerate: "{install}", When: always,
		Explain: "Installed dependencies. Your package manager recreates them from the lockfile."},
	{Kind: "next", Rel: ".next", Level: Safe, Regenerate: "next dev / next build", When: anyOf(dep("next"), cfg("next.config")),
		Explain: "Next.js build output and caches, including the Turbopack caches in .next/cache and .next/dev/cache, which Next.js does not prune."},
	{Kind: "open-next", Rel: ".open-next", Level: Review, Regenerate: "opennextjs-cloudflare build / open-next build", When: dep("@opennextjs/cloudflare", "@opennextjs/aws"),
		Explain: "OpenNext's deployable build of a Next.js app. Rebuildable, but it may be what you deploy."},
	{Kind: "nuxt", Rel: ".nuxt", Level: Safe, Regenerate: "nuxt dev / nuxt build (nuxi cleanup removes it too)", When: anyOf(dep("nuxt"), cfg("nuxt.config")),
		Explain: "Nuxt's generated build directory."},
	{Kind: "nitro", Rel: ".nitro", Level: Safe, Regenerate: "nuxt dev / nitro dev", When: anyOf(dep("nuxt", "nitropack", "nitro"), cfg("nuxt.config", "nitro.config")),
		Explain: "Nitro's dev build directory. Nitro's dev storage in .data is never touched."},
	{Kind: "nuxt-output", Rel: ".output", Level: Review, Regenerate: "the framework's build (nuxt build, vinxi build, …)",
		When:    anyOf(dep("nuxt", "nitropack", "nitro", "vinxi", "@solidjs/start", "@tanstack/react-start", "@tanstack/solid-start"), cfg("nuxt.config", "nitro.config")),
		Explain: "Nitro production bundle (Nuxt, SolidStart, TanStack Start). Rebuildable, but it may be what you deploy."},
	{Kind: "sveltekit", Rel: ".svelte-kit", Level: Safe, Regenerate: "svelte-kit sync / vite dev", When: anyOf(dep("@sveltejs/kit"), cfg("svelte.config")),
		Explain: "SvelteKit's generated types and build output."},
	{Kind: "astro", Rel: ".astro", Level: Review, Regenerate: "astro sync / astro dev", When: anyOf(dep("astro"), cfg("astro.config")),
		Explain: "Astro's generated types and content cache. It also keeps your dev-toolbar settings (settings.json), which are lost."},
	{Kind: "angular-cache", Rel: ".angular/cache", Level: Safe, Regenerate: "ng build / ng serve (ng cache clean removes it too)", When: cfg("angular.json"),
		Explain: "Angular CLI build cache."},
	{Kind: "turbo", Rel: ".turbo/cache", Level: Safe, Regenerate: "the next turbo run (cache misses once)", When: turbo,
		Explain: "Turborepo's local task cache. The rest of .turbo (remote-cache link from turbo link, TUI preferences) is never touched."},
	{Kind: "nx-cache", Rel: ".nx/cache", Level: Safe, Regenerate: "the next nx run (nx reset removes it too)", When: cfg("nx.json"),
		Explain: "Nx's local task cache."},
	{Kind: "nx-workspace-data", Rel: ".nx/workspace-data", Level: Safe, Regenerate: "the next nx command (nx reset removes it too)", When: cfg("nx.json"),
		Explain: "Nx project graph and daemon data."},
	{Kind: "moon-cache", Rel: ".moon/cache", Level: Safe, Regenerate: "the next moon run", When: at(".moon/workspace.yml", ".moon/workspace.yaml", ".moon/workspace.pkl"),
		Explain: "moon's task cache and hashes."},
	{Kind: "parcel-cache", Rel: ".parcel-cache", Level: Safe, Regenerate: "the next parcel build", When: anyOf(dep("parcel"), cfg(".parcelrc")),
		Explain: "Parcel's build cache."},
	{Kind: "expo", Rel: ".expo", Level: Safe, Regenerate: "expo start", When: dep("expo"),
		Explain: "Expo's per-machine dev state (device list, dev-server settings)."},
	{Kind: "docusaurus", Rel: ".docusaurus", Level: Safe, Regenerate: "docusaurus start / build (docusaurus clear removes it too)", When: cfg("docusaurus.config"),
		Explain: "Docusaurus generated files."},
	{Kind: "vitepress-cache", Rel: ".vitepress/cache", Level: Safe, Regenerate: "vitepress dev / build", When: vitepress,
		Explain: "VitePress's dev-server dependency cache."},
	{Kind: "vitepress-cache", Rel: "docs/.vitepress/cache", Level: Safe, Regenerate: "vitepress dev / build", When: vitepress,
		Explain: "VitePress's dev-server dependency cache."},
	{Kind: "vitepress-dist", Rel: ".vitepress/dist", Level: Review, Regenerate: "vitepress build", When: vitepress,
		Explain: "VitePress static site. Rebuildable, but it may be what you publish."},
	{Kind: "vitepress-dist", Rel: "docs/.vitepress/dist", Level: Review, Regenerate: "vitepress build", When: vitepress,
		Explain: "VitePress static site. Rebuildable, but it may be what you publish."},
	{Kind: "react-router", Rel: ".react-router", Level: Safe, Regenerate: "react-router typegen / dev", When: anyOf(dep("@react-router/dev"), cfg("react-router.config")),
		Explain: "React Router generated route types."},
	{Kind: "vinxi", Rel: ".vinxi", Level: Safe, Regenerate: "vinxi dev / build", When: dep("vinxi"),
		Explain: "Vinxi build cache (SolidStart, older TanStack Start)."},
	{Kind: "tanstack-tmp", Rel: ".tanstack/tmp", Level: Safe, Regenerate: "the TanStack router generator", When: func(p *Project) bool { return p.hasPrefix("@tanstack/") },
		Explain: "TanStack Router/Start generator scratch files."},
	{Kind: "wrangler-tmp", Rel: ".wrangler/tmp", Level: Safe, Regenerate: "wrangler dev", When: wrangler,
		Explain: "Wrangler build scratch. Local D1/KV/R2 data in .wrangler/state is never touched."},
	{Kind: "wrangler-cache", Rel: ".wrangler/cache", Level: Safe, Regenerate: "wrangler dev", When: wrangler,
		Explain: "Wrangler cache. Local D1/KV/R2 data in .wrangler/state is never touched."},
	{Kind: "vercel-output", Rel: ".vercel/output", Level: Review, Regenerate: "vercel build", When: anyOf(dep("vercel"), cfg("vercel.json"), func(p *Project) bool { return p.file(".vercel") }),
		Explain: "Prebuilt Vercel output. The project link and pulled env files in .vercel are never touched."},
	{Kind: "netlify-functions", Rel: ".netlify/functions-serve", Level: Safe, Regenerate: "netlify dev", When: anyOf(tool("netlify-cli"), cfg("netlify.toml")),
		Explain: "Functions Netlify Dev bundled locally. The site link, blobs and local database in .netlify are never touched."},
	{Kind: "serverless", Rel: ".serverless", Level: Review, Regenerate: "serverless package / deploy", When: cfg("serverless"),
		Explain: "Serverless Framework packaging output. Rebuildable, but it may be what you deploy."},
	{Kind: "sst-platform", Rel: ".sst/platform", Level: Safe, Regenerate: "the next sst command", When: cfg("sst.config"),
		Explain: "SST's generated platform code. Stage state in .sst/stage is never touched."},
	{Kind: "sst-log", Rel: ".sst/log", Level: Safe, Regenerate: "the next sst command", When: cfg("sst.config"),
		Explain: "SST's local logs."},
	{Kind: "cdk-out", Rel: "cdk.out", Level: Safe, Regenerate: "cdk synth", When: allOf(cfg("cdk.json"), dep("aws-cdk-lib")),
		Explain: "AWS CDK's synthesized CloudFormation templates and assets."},
	{Kind: "mastra-build", Rel: ".mastra/.build", Level: Safe, Regenerate: "mastra dev / mastra build", When: dep("mastra", "@mastra/core"),
		Explain: "Mastra's intermediate build files. .mastra/output (which can hold a local database) is never touched."},
	{Kind: "medusa-server", Rel: ".medusa/server", Level: Review, Regenerate: "medusa build", When: dep("@medusajs/medusa", "@medusajs/framework"),
		Explain: "Medusa's production build. Rebuildable, but it may be what you deploy."},
	{Kind: "strapi", Rel: ".strapi", Level: Review, Regenerate: "strapi develop / build", When: dep("@strapi/strapi"),
		Explain: "Strapi's generated admin build. The database in .tmp is never touched."},
	{Kind: "gatsby-cache", Rel: ".cache", Level: Safe, Regenerate: "gatsby develop / build (gatsby clean removes it too)", When: cfg("gatsby-config"),
		Explain: "Gatsby's build cache."},
	{Kind: "firebase", Rel: ".firebase", Level: Safe, Regenerate: "the next firebase deploy", When: cfg("firebase.json"),
		Explain: "Firebase Hosting's deploy cache (file hashes of the last upload); the next deploy uploads everything again."},
	{Kind: "yarn-unplugged", Rel: ".yarn/unplugged", Level: Safe, Regenerate: "yarn install", When: cfg(".yarnrc.yml"),
		Explain: "Yarn Plug'n'Play unpacked packages. The committed .yarn/cache is never touched."},
	{Kind: "nyc-output", Rel: ".nyc_output", Level: Safe, Regenerate: "the next nyc run", When: tool("nyc"),
		Explain: "nyc coverage raw data."},
	{Kind: "coverage", Rel: "coverage", Level: Review, Generic: true, Regenerate: "run the tests with coverage",
		When:    anyOf(tool("jest", "vitest", "nyc", "c8"), cfg("vitest.config", "jest.config"), func(p *Project) bool { return p.hasPrefix("@vitest/coverage") }),
		Explain: "Coverage report. Rebuildable by running the tests."},
	{Kind: "playwright-results", Rel: "test-results", Level: Safe, Generic: true, Regenerate: "the next playwright test run (it empties the folder first)", When: playwright,
		Explain: "Playwright's per-test output (traces, screenshots, videos), cleared at the start of every run."},
	{Kind: "playwright-blob", Rel: "blob-report", Level: Safe, Generic: true, Regenerate: "the next playwright test run with the blob reporter", When: playwright,
		Explain: "Playwright's blob report, replaced on every run."},
	{Kind: "playwright-report", Rel: "playwright-report", Level: Review, Generic: true, Regenerate: "the next playwright test run", When: playwright,
		Explain: "Playwright's HTML report of the last run."},
	{Kind: "cypress-videos", Rel: "cypress/videos", Level: Safe, Regenerate: "the next cypress run", When: cypress,
		Explain: "Cypress run videos, trashed before every run by default."},
	{Kind: "cypress-screenshots", Rel: "cypress/screenshots", Level: Safe, Regenerate: "the next cypress run", When: cypress,
		Explain: "Cypress failure screenshots, trashed before every run by default."},
	{Kind: "cypress-downloads", Rel: "cypress/downloads", Level: Safe, Regenerate: "the next cypress run", When: cypress,
		Explain: "Files downloaded during Cypress tests, trashed before every run by default."},
	{Kind: "storybook-static", Rel: "storybook-static", Level: Review, Generic: true, Regenerate: "storybook build", When: anyOf(tool("storybook"), cfg(".storybook")),
		Explain: "Static Storybook build. Rebuildable, but it may be what you publish."},
	{Kind: "rn-pods", Rel: "ios/Pods", Level: Review, Regenerate: "pod install (npx pod-install)", When: allOf(native, at("ios/Podfile")),
		Explain: "CocoaPods dependencies of the iOS app. Some teams commit them; tracked Pods are never touched."},
	{Kind: "rn-ios-build", Rel: "ios/build", Level: Safe, Regenerate: "the next iOS build", When: allOf(native, at("ios/Podfile")),
		Explain: "Xcode build output of the iOS app."},
	{Kind: "rn-vendor-bundle", Rel: "vendor/bundle", Level: Safe, Regenerate: "bundle install", When: allOf(native, at("Gemfile")),
		Explain: "Ruby gems (CocoaPods) Bundler installed for the iOS build."},
	{Kind: "capacitor-pods", Rel: "ios/App/Pods", Level: Review, Regenerate: "npx cap sync ios", When: allOf(capacitor, at("ios/App/Podfile")),
		Explain: "CocoaPods dependencies of the Capacitor iOS app. Tracked Pods are never touched."},
	{Kind: "capacitor-ios-build", Rel: "ios/App/build", Level: Safe, Regenerate: "the next iOS build", When: allOf(capacitor, at("ios/App/Podfile")),
		Explain: "Xcode build output of the Capacitor iOS app."},
	{Kind: "capacitor-derived-data", Rel: "ios/DerivedData", Level: Safe, Regenerate: "the next iOS build", When: allOf(capacitor, at("ios/App")),
		Explain: "Xcode DerivedData kept next to the Capacitor iOS app."},
	{Kind: "rn-android-build", Rel: "android/app/build", Level: Safe, Regenerate: "the next Android build", When: allOf(native, androidProject),
		Explain: "Gradle build output of the Android app."},
	{Kind: "capacitor-android-build", Rel: "android/app/build/intermediates", Level: Review, Regenerate: "the next Android build", When: allOf(capacitor, androidProject),
		Explain: "Gradle's intermediate build files of the Capacitor Android app (android/app/build keeps a tracked .npmkeep, so only this part is offered)."},
	{Kind: "android-root-build", Rel: "android/build", Level: Safe, Regenerate: "the next Android build", When: allOf(anyOf(native, capacitor), androidProject),
		Explain: "Gradle's root build output of the Android app."},
	{Kind: "rn-android-cxx", Rel: "android/app/.cxx", Level: Safe, Regenerate: "the next Android build", When: allOf(native, androidProject),
		Explain: "Native (C++/CMake) build output of the Android app."},
	{Kind: "rn-android-gradle", Rel: "android/.gradle", Level: Safe, Regenerate: "the next Gradle run", When: allOf(anyOf(native, capacitor), androidProject),
		Explain: "Gradle's project cache for the Android app."},
	{Kind: "tauri-target", Rel: "src-tauri/target", Level: Review, Regenerate: "tauri dev / tauri build (cargo rebuilds it, slowly)", When: allOf(tool("@tauri-apps/cli"), at("src-tauri/Cargo.toml")),
		Explain: "Rust build output of the Tauri app, often several GB. target/release/bundle holds the installers you built."},
	{Kind: "hardhat-cache", Rel: "cache", Level: Safe, Generic: true, Regenerate: "hardhat compile", When: allOf(hardhat, not(foundry)),
		Explain: "Hardhat's compilation cache."},
	{Kind: "hardhat-artifacts", Rel: "artifacts", Level: Review, Generic: true, Regenerate: "hardhat compile", When: allOf(hardhat, not(foundry)),
		Explain: "Hardhat's compiled contracts. Rebuildable, but scripts may read them. ignition/deployments is never touched."},
	{Kind: "dist", Rel: "dist", Level: Review, Generic: true, Regenerate: "your build script",
		When: anyOf(dep("vite", "astro", "parcel", "tsup", "tsdown", "electron-builder", "@angular/core", "rollup", "webpack", "expo",
			"@rsbuild/core", "@rspack/cli", "@rspack/core", "@rslib/core"), cfg("vite.config", "astro.config", "angular.json", "electron-builder", "rsbuild.config", "rspack.config", "rslib.config", "tsdown.config", "tsup.config")),
		Explain: "Build output. Rebuildable, but it may be what you publish or deploy."},
	{Kind: "build", Rel: "build", Level: Review, Generic: true, Regenerate: "your build script", When: anyOf(dep("react-scripts", "@docusaurus/core", "@react-router/dev", "@remix-run/dev", "@sveltejs/adapter-static", "@sveltejs/adapter-node"), cfg("docusaurus.config", "react-router.config", "remix.config")),
		Explain: "Build output. Rebuildable, but it may be what you publish or deploy."},
	{Kind: "eleventy-site", Rel: "_site", Level: Review, Generic: true, Regenerate: "eleventy", When: anyOf(dep("@11ty/eleventy"), cfg("eleventy.config", ".eleventy")),
		Explain: "Eleventy's generated site. Rebuildable, but it may be what you publish."},
	{Kind: "out", Rel: "out", Level: Review, Generic: true, Regenerate: "next build (static export) / electron-forge make / electron-vite build",
		When:    allOf(not(remotion), not(foundry), anyOf(nextExport, cfg("forge.config"), dep("@electron-forge/cli", "electron-vite"))),
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
	".turbo": true, ".nx": true, ".moon": true, ".sst": true, ".mastra": true, ".medusa": true, ".open-next": true,
	".serverless": true, ".vitepress": true, ".astro": true, ".nitro": true, ".strapi": true, "cdk.out": true,
	"test-results": true, "playwright-report": true, "blob-report": true, ".parcel-cache": true, ".angular": true, ".expo": true, ".docusaurus": true,
	".react-router": true, ".vinxi": true, ".tanstack": true, ".wrangler": true, ".vercel": true, ".netlify": true,
	".yarn": true, ".cache": true, ".firebase": true, ".pnpm-store": true, "Pods": true, "DerivedData": true,
	".gradle": true, ".nyc_output": true, "storybook-static": true, ".Trash": true, ".svn": true, ".hg": true,
	"bower_components": true, "jspm_packages": true, ".venv": true, "__pycache__": true, "target": true,
}
