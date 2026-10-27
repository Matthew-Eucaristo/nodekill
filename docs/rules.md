# Sources for the catalog

Every rule and command in nodekill comes from the owning tool's documentation or source. A rule matches a folder only at its exact position below `package.json`, and only when a marker proves the project uses the tool. Test runners and build tools also count when they are hoisted to the workspace root. Rules whose level rests on inference rather than a source are capped at review. Checked 2026-10.

## Project folders

| Kind | Path | Marker | Level | Source |
|---|---|---|---|---|
| `node_modules` | `node_modules` | `package.json` | safe | [npm folders](https://docs.npmjs.com/cli/v12/configuring-npm/folders), [pnpm node-modules](https://pnpm.io/settings/node-modules), [Bun install](https://bun.com/docs/pm/cli/install) |
| `next` | `.next` | `next` dep or `next.config.*` | safe; review with `output: 'standalone'` | [distDir](https://nextjs.org/docs/app/api-reference/config/next-config-js/distDir), [Turbopack filesystem cache](https://nextjs.org/docs/app/api-reference/config/next-config-js/turbopackFileSystemCache). The Turbopack caches live in `.next/cache/turbopack` and `.next/dev/cache/turbopack`, and Next.js does not prune them ([next.js#81161](https://github.com/vercel/next.js/issues/81161)) |
| `open-next` | `.open-next` | `@opennextjs/cloudflare`, `@opennextjs/aws` | review | [OpenNext Cloudflare](https://opennext.js.org/cloudflare/get-started) |
| `out` | `out` | Next `output: 'export'`, `forge.config.*`, `@electron-forge/cli`, `electron-vite`; never with Remotion or Foundry | review | [static exports](https://nextjs.org/docs/app/guides/static-exports), [Forge configuration](https://www.electronforge.io/config/configuration), [electron-vite build](https://electron-vite.org/guide/build) |
| `nuxt` | `.nuxt` | `nuxt` / `nuxt.config.*` | safe | [.nuxt](https://nuxt.com/docs/4.x/directory-structure/nuxt), [nuxi cleanup](https://nuxt.com/docs/4.x/api/commands/cleanup) |
| `nitro` | `.nitro` | nuxt, nitropack, nitro | safe | Nitro `config/defaults.ts` (`buildDir: ".nitro"`). Its dev storage `.data` is never a candidate |
| `nuxt-output` | `.output` | Nuxt, Nitro, Vinxi, SolidStart, TanStack Start | review | [.output](https://nuxt.com/docs/4.x/directory-structure/output) |
| `sveltekit` | `.svelte-kit` | `@sveltejs/kit` / `svelte.config.*` | safe | [SvelteKit configuration](https://svelte.dev/docs/kit/configuration) |
| `astro` | `.astro` | `astro` / `astro.config.*` | review (it also keeps `settings.json` preferences) | [Astro CLI](https://docs.astro.build/en/reference/cli-reference/) |
| `angular-cache` | `.angular/cache` | `angular.json` | safe | [workspace config](https://angular.dev/reference/configs/workspace-config), [ng cache clean](https://angular.dev/cli/cache/clean) |
| `turbo` | `.turbo/cache` | `turbo` (workspace root too) / `turbo.json(c)` | safe | [Turborepo configuration](https://turborepo.dev/docs/reference/configuration) (`cacheDir`). The rest of `.turbo` holds the `turbo link` remote-cache config and TUI preferences, and is never a candidate |
| `nx-cache`, `nx-workspace-data` | `.nx/cache`, `.nx/workspace-data` | `nx.json` | safe | nx `packages/nx/src/utils/cache-directory.ts`, [nx reset](https://nx.dev/docs/reference/nx-commands) |
| `moon-cache` | `.moon/cache` | `.moon/workspace.*` | safe | [moon cache](https://moonrepo.dev/docs/concepts/cache) |
| `parcel-cache` | `.parcel-cache` | `parcel` / `.parcelrc` | safe | [Parcel CLI](https://parceljs.org/features/cli/) |
| `expo` | `.expo` | `expo` | safe | [Expo CLI](https://docs.expo.dev/more/expo-cli/) |
| `docusaurus` | `.docusaurus` | `docusaurus.config.*` | safe | [docusaurus clear](https://docusaurus.io/docs/cli) |
| `vitepress-cache`, `vitepress-dist` | `.vitepress/cache`, `.vitepress/dist` (also below `docs/`) | `vitepress` | safe; review | [site config](https://vitepress.dev/reference/site-config) |
| `react-router` | `.react-router` | `@react-router/dev` / `react-router.config.*` | safe | [type safety](https://reactrouter.com/explanation/type-safety) |
| `vinxi`, `tanstack-tmp` | `.vinxi`, `.tanstack/tmp` | `vinxi`, `@tanstack/*` | safe | TanStack Router generator (observed). Git evidence protects it when it is not ignored |
| `wrangler-tmp`, `wrangler-cache` | `.wrangler/tmp`, `.wrangler/cache` | `wrangler` / `wrangler.*` | safe | [Wrangler local data](https://developers.cloudflare.com/workers/local-development/local-data/). `.wrangler/state` holds local D1/KV/R2 data and is never a candidate |
| `vercel-output` | `.vercel/output` | `vercel` / `vercel.json` / `.vercel` | review | [vercel build](https://vercel.com/docs/cli/build). The rest of `.vercel` (project link, pulled env) is never a candidate |
| `netlify-functions` | `.netlify/functions-serve` | `netlify-cli` / `netlify.toml` | safe | netlify/cli `src/utils/functions/functions.ts`. `.netlify/state.json`, blobs and the local database are never candidates |
| `serverless` | `.serverless` | `serverless.*` | review | [serverless package](https://www.serverless.com/framework/docs/providers/aws/cli-reference/package) |
| `sst-platform`, `sst-log` | `.sst/platform`, `.sst/log` | `sst.config.*` | safe | sst `pkg/project/path/path.go`. `.sst/stage` (your personal stage) is never a candidate |
| `cdk-out` | `cdk.out` | `cdk.json` and `aws-cdk-lib` | safe | aws-cdk-cli TypeScript init template `.gitignore` |
| `mastra-build` | `.mastra/.build` | `mastra`, `@mastra/core` | safe | [Building Mastra](https://mastra.ai/docs/deployment/building-mastra). `.mastra/output` can hold a local database and is never a candidate |
| `medusa-server` | `.medusa/server` | `@medusajs/medusa`, `@medusajs/framework` | review | [Medusa build](https://docs.medusajs.com/learn/build) |
| `strapi` | `.strapi` | `@strapi/strapi` | review | Strapi template `.gitignore`. `.tmp` holds the default SQLite database and is never a candidate |
| `gatsby-cache` | `.cache` | `gatsby-config.*` | safe | [gatsby clean](https://www.gatsbyjs.com/docs/reference/gatsby-cli/) |
| `firebase` | `.firebase` | `firebase.json` | safe | Firebase Hosting's deploy cache (`.firebase/hosting.*.cache`), which the next `firebase deploy` rewrites |
| `yarn-unplugged` | `.yarn/unplugged` | `.yarnrc.yml` | safe | [Yarn Q&A: what to gitignore](https://yarnpkg.com/getting-started/qa). `.yarn/cache` (Zero-Installs) is never a candidate |
| `nyc-output`, `coverage` | `.nyc_output`, `coverage` | `nyc`, `jest`, `vitest`, `c8`, `vitest.config.*`, `jest.config.*` | safe; review | [nyc](https://github.com/istanbuljs/nyc) |
| `playwright-results`, `playwright-blob` | `test-results`, `blob-report` | `@playwright/test` / `playwright.config.*` | safe | [TestConfig.outputDir](https://playwright.dev/docs/api/class-testconfig) ("cleaned at the start"), [reporters](https://playwright.dev/docs/test-reporters) |
| `playwright-report` | `playwright-report` | same | review | [reporters](https://playwright.dev/docs/test-reporters). `playwright/.auth` (saved logins) is never a candidate |
| `cypress-*` | `cypress/videos`, `cypress/screenshots`, `cypress/downloads` | `cypress` / `cypress.config.*` | safe | [Cypress configuration](https://docs.cypress.io/app/references/configuration) (`trashAssetsBeforeRuns`) |
| `storybook-static` | `storybook-static` | `storybook` / `.storybook` | review | [publish Storybook](https://storybook.js.org/docs/sharing/publish-storybook) |
| React Native | `ios/Pods` (review), `ios/build`, `vendor/bundle`, `android/app/build`, `android/app/.cxx`, `android/build`, `android/.gradle` | `react-native`/`expo` with a Podfile, Gemfile or Gradle build | safe unless noted | [react-native-community/template `_gitignore`](https://github.com/react-native-community/template/blob/main/template/_gitignore). `ios/` and `android/` themselves are never candidates ([Expo prebuild](https://docs.expo.dev/more/expo-cli/)) |
| Capacitor | `ios/App/Pods` (review), `ios/App/build`, `ios/DerivedData`, `android/app/build/intermediates` (review), `android/build`, `android/.gradle` | `@capacitor/core`, `@capacitor/cli`, `capacitor.config.*` | safe unless noted | capacitor `ios-pods-template/.gitignore`, `android-template/.gitignore`. `android/app/build` keeps a tracked `.npmkeep`, so only `intermediates` is offered |
| `tauri-target` | `src-tauri/target` | `@tauri-apps/cli` and `src-tauri/Cargo.toml` | review (slow to rebuild; `release/bundle` holds installers) | create-tauri-app `templates/_base_/src-tauri/_gitignore` |
| `hardhat-cache`, `hardhat-artifacts` | `cache`, `artifacts` | `hardhat.config.*` without `foundry.toml` | safe; review (generic names: protected outside git) | [Hardhat configuration](https://hardhat.org/docs/reference/configuration). `ignition/deployments` is never a candidate |
| `dist`, `build` | `dist`, `build` | a builder that writes them (Vite, Astro, Rsbuild, Rspack, Rslib, tsdown, tsup, Expo export, Docusaurus, …) | review; protected outside git | [Rsbuild distPath](https://rsbuild.rs/config/output/dist-path), [Rspack output](https://rspack.rs/config/output), [tsdown](https://tsdown.dev/options/output-directory), [Expo CLI](https://docs.expo.dev/more/expo-cli/) |
| `eleventy-site` | `_site` | `@11ty/eleventy` / `eleventy.config.*` | review | [Eleventy config](https://www.11ty.dev/docs/config/) |
| `remotion-out` | `out` | `remotion` | protected | [Remotion render](https://www.remotion.dev/docs/cli/render): render output, never removed |

Caches inside `node_modules` (`.cache`, `.vite`, `.astro`, `.prisma`) are part of the `node_modules` item.

## Machine-wide items

Every command runs from your home folder. That way it acts on the configuration the scan showed, never a project's `.npmrc`, `.yarnrc.yml` or bunfig. Commands also run with prompts, update checks, Corepack downloads, pnpm version switching and Homebrew autoremove turned off (`QuietEnv` in `internal/scan/env.go`). A command that empties a folder runs only when that folder is a real directory at least two levels inside home.

| Item | Location (macOS default) | Command | The tool's own advice | Source |
|---|---|---|---|---|
| npm cache | `npm config get cache` → `_cacache` | `npm cache clean --force` | npm calls the cache self-healing and suggests `npm cache verify` for problems; cleaning only reclaims space | [npm-cache](https://docs.npmjs.com/cli/v12/commands/npm-cache) |
| npx entries | `<cache>/_npx/<hash>` | `npm cache npx rm <hash>` (npm ≥ 11.2.0; older npm: Trash) | npx downloads them again when needed | npm `lib/commands/cache.js`. Last run is the folder's mtime (`concurrency.lock`, npm ≥ 11.6.1, [npm/cli#8512](https://github.com/npm/cli/pull/8512)) |
| pnpm store | `pnpm store path` (default `$PNPM_HOME/store`, `$XDG_DATA_HOME/pnpm/store`, `~/Library/pnpm/store`) | `pnpm store prune` | prune is safe, but not too often: removed packages download again | [pnpm store](https://pnpm.io/cli/store), [packageImportMethod](https://pnpm.io/settings/other) |
| Yarn 1 cache | `$YARN_CACHE_FOLDER`, `$XDG_CACHE_HOME/yarn`, `~/Library/Caches/Yarn` | `yarn cache clean` (removes the whole root) | | yarn `src/cli/commands/cache.js`, `src/util/user-dirs.js` |
| Yarn 2+ global cache | `~/.yarn/berry/cache` | `yarn cache clean`, only when `yarn config get cacheFolder` from home is this folder | Zero-Installs projects keep their own `.yarn/cache` and are not affected | [yarn cache clean](https://yarnpkg.com/cli/cache/clean), berry `cache/clean.ts` |
| Bun cache | `bun pm cache` (`$BUN_INSTALL_CACHE_DIR`, `$BUN_INSTALL/install/cache`) | `bun pm cache rm --cache-dir <dir>` | `bun pm cache prune` (older than 30 days) is the gentler option | [bun pm cache](https://bun.com/docs/pm/cli/pm) |
| Deno cache | `deno info --json` → `denoDir` | `deno clean`. Review, because it also deletes the REPL history. Protected when `location_data` holds localStorage/KV data or the folder holds Deno's install root | `deno clean` removes the whole DENO_DIR | [deno clean](https://docs.deno.com/runtime/reference/cli/clean/), deno `cli/tools/clean.rs` |
| Corepack cache | `$COREPACK_HOME`, `$XDG_CACHE_HOME/node/corepack`, `~/.cache/node/corepack` → `v1` | `corepack cache clean` (Corepack ≥ 0.25) | | [corepack](https://github.com/nodejs/corepack) |
| node-gyp headers | `~/Library/Caches/node-gyp/<version>` (devdir overrides honoured) | `node-gyp remove <version>`, run through npm's bundled node-gyp | | node-gyp `lib/remove.js` |
| nvm download cache | `$NVM_DIR/.cache` | `nvm cache clear` | | [nvm](https://github.com/nvm-sh/nvm) |
| Global packages | `npm root -g`, `pnpm ls -g --json`, `~/.bun/install/global`, Yarn 1 `global` folder, `~/.deno/bin` launchers | `npm uninstall -g`, `pnpm remove -g`, `bun remove -g`, `yarn global remove` (Yarn 1), `deno uninstall -g` (Deno ≥ 2; only launchers `deno install` wrote) | npm and corepack are protected | [npm uninstall](https://docs.npmjs.com/cli/v12/commands/npm-uninstall), [pnpm remove](https://pnpm.io/cli/remove), [bun remove](https://bun.com/docs/pm/cli/remove), [deno uninstall](https://docs.deno.com/runtime/reference/cli/uninstall/) |
| nvm | `$NVM_DIR/versions/node/*` | `nvm uninstall <v>` (via `nvm.sh`, also Homebrew's) | | [nvm](https://github.com/nvm-sh/nvm) |
| fnm · mise · n · asdf · nodenv | per tool | `fnm uninstall`, `mise uninstall node@<v>`, `n rm`, `asdf uninstall nodejs`, `nodenv uninstall -f` | | [fnm](https://github.com/Schniz/fnm/blob/master/docs/commands.md), [mise](https://mise.jdx.dev/cli/uninstall.html), each tool's README |
| Homebrew | `brew --cellar node@XX` | `brew uninstall node@XX`, with `HOMEBREW_NO_AUTOREMOVE=1`. Protected while `brew uses --installed` lists dependents | | [Homebrew FAQ](https://docs.brew.sh/FAQ), [brew man page](https://docs.brew.sh/Manpage) |
| Volta, nvm-windows, nvs, proto, nvm.fish | per tool | Listed with the command for you to run; nodekill does not run it. Volta cannot uninstall Node versions. The others' non-interactive uninstall (or nvm-windows 2.x output) is not verified | | [Volta source](https://github.com/volta-cli/volta), each tool's README |
| TypeScript, Electron, electron-builder, Playwright, Cypress, Puppeteer caches | `~/Library/Caches/…`, `~/.cache/puppeteer` | none for the whole folder, so it moves to the Trash | | [Playwright browsers](https://playwright.dev/docs/browsers), [Cypress cache](https://docs.cypress.io/app/references/command-line), [Puppeteer](https://pptr.dev/browsers-api), [electron-builder](https://www.electron.build/docs/tutorials/offline-air-gapped-builds) |

Playwright's, Cypress's and Puppeteer's own commands are not used. Run through `npx` from outside a project, they act on the wrong installation: Cypress `cache prune` keeps only the version running it, and `playwright uninstall` without `--all` uses the invoking install.
