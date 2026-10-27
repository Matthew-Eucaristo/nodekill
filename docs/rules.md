# Sources for the catalog

Every rule and command in nodekill comes from the owning tool's documentation or source. Rules match a folder only at its position below `package.json` and only when the marker proves the tool is used. Checked 2026-10.

## Project folders

| Kind | Path | Marker | Source |
|---|---|---|---|
| `node_modules` | `node_modules` | `package.json` | [npm folders](https://docs.npmjs.com/cli/v12/configuring-npm/folders), [pnpm node-modules](https://pnpm.io/settings/node-modules), [Bun install](https://bun.com/docs/pm/cli/install) |
| `next` | `.next` | `next` dep / `next.config.*` | [distDir](https://nextjs.org/docs/app/api-reference/config/next-config-js/distDir), [Turbopack filesystem cache](https://nextjs.org/docs/app/api-reference/config/next-config-js/turbopackFileSystemCache) (`.next/cache/turbopack`, `.next/dev/cache/turbopack`; Next.js does not prune them: [next.js#81161](https://github.com/vercel/next.js/issues/81161)); `output: 'standalone'` → review |
| `out` | `out` | Next `output: 'export'`, `forge.config.*` | [static exports](https://nextjs.org/docs/app/guides/static-exports) |
| `nuxt`, `nuxt-output` | `.nuxt`, `.output` | `nuxt` / `nuxt.config.*` | [.nuxt](https://nuxt.com/docs/4.x/directory-structure/nuxt), [.output](https://nuxt.com/docs/4.x/directory-structure/output), [nuxi cleanup](https://nuxt.com/docs/4.x/api/commands/cleanup) |
| `sveltekit` | `.svelte-kit` | `@sveltejs/kit` / `svelte.config.*` | [SvelteKit configuration](https://svelte.dev/docs/kit/configuration) |
| `angular-cache` | `.angular/cache` | `angular.json` | [workspace config](https://angular.dev/reference/configs/workspace-config), [ng cache clean](https://angular.dev/cli/cache/clean) |
| `turbo` | `.turbo` | `turbo` / `turbo.json` | [Turborepo configuration](https://turborepo.dev/docs/reference/configuration) |
| `nx-cache`, `nx-workspace-data` | `.nx/cache`, `.nx/workspace-data` | `nx.json` | [Nx environment variables](https://nx.dev/docs/reference/environment-variables), [nx reset](https://nx.dev/docs/reference/nx-commands) |
| `parcel-cache` | `.parcel-cache` | `parcel` / `.parcelrc` | [Parcel CLI](https://parceljs.org/features/cli/) |
| `expo` | `.expo` | `expo` | [Expo CLI](https://docs.expo.dev/more/expo-cli/) |
| `docusaurus` | `.docusaurus` | `docusaurus.config.*` | [docusaurus clear](https://docusaurus.io/docs/cli) |
| `react-router` | `.react-router` | `@react-router/dev` / `react-router.config.*` | [type safety](https://reactrouter.com/explanation/type-safety) |
| `vinxi`, `tanstack-tmp` | `.vinxi`, `.tanstack/tmp` | `vinxi`, `@tanstack/*` | TanStack Router generator (observed); git evidence protects it when not ignored |
| `wrangler-tmp`, `wrangler-cache` | `.wrangler/tmp`, `.wrangler/cache` | `wrangler` / `wrangler.*` | [Wrangler local data](https://developers.cloudflare.com/workers/local-development/local-data/): `.wrangler/state` holds local D1/KV/R2 data and is never a candidate |
| `vercel-output` | `.vercel/output` | `vercel` / `vercel.json` / `.vercel` | [vercel build](https://vercel.com/docs/cli/build); the rest of `.vercel` (project link, pulled env) is never a candidate |
| `gatsby-cache` | `.cache` | `gatsby-config.*` | [gatsby clean](https://www.gatsbyjs.com/docs/reference/gatsby-cli/) |
| `firebase` | `.firebase` | `firebase.json` | Firebase Hosting upload cache |
| `yarn-unplugged` | `.yarn/unplugged` | `.yarnrc.yml` | [Yarn Q&A: what to gitignore](https://yarnpkg.com/getting-started/qa); `.yarn/cache` is never a candidate (zero-installs) |
| `nyc-output`, `coverage` | `.nyc_output`, `coverage` | `nyc`, `jest`, `vitest`, `c8` | [nyc](https://github.com/istanbuljs/nyc) |
| `storybook-static` | `storybook-static` | `storybook` / `.storybook` | [publish Storybook](https://storybook.js.org/docs/sharing/publish-storybook) |
| `rn-*` | `ios/Pods`, `ios/build`, `android/app/build`, `android/.gradle` | `react-native`/`expo` + Podfile / Gradle build | `ios/` and `android/` themselves are never candidates ([Expo prebuild](https://docs.expo.dev/more/expo-cli/)) |
| `dist`, `build` | `dist`, `build` | a builder that writes them | review; protected outside git |
| `remotion-out` | `out` | `remotion` | [Remotion render](https://www.remotion.dev/docs/cli/render): render output, always protected |

## Machine-wide items

| Item | Location (macOS) | Command | Source |
|---|---|---|---|
| npm cache | `npm config get cache`/`_cacache` | `npm cache clean --force` | [npm-cache](https://docs.npmjs.com/cli/v12/commands/npm-cache) |
| npx entries | `<cache>/_npx/<hash>` | `npm cache npx rm <hash>` | npm `lib/commands/cache.js`; last run = folder mtime (`concurrency.lock`, npm ≥ 11.6.1, [npm/cli#8512](https://github.com/npm/cli/pull/8512)) |
| pnpm store | `pnpm store path` | `pnpm store prune` | [pnpm store](https://pnpm.io/cli/store), [packageImportMethod](https://pnpm.io/settings/other) (clone first on macOS) |
| Yarn 1 cache | `yarn cache dir` | `yarn cache clean` | [Yarn 1 cache](https://classic.yarnpkg.com/en/docs/cli/cache) |
| Yarn 2+ cache | `~/.yarn/berry/cache` | `yarn cache clean --all` | [yarn cache clean](https://yarnpkg.com/cli/cache/clean), [berry source](https://github.com/yarnpkg/berry/blob/master/packages/plugin-essentials/sources/commands/cache/clean.ts) |
| Bun cache | `bun pm cache` | `bun pm cache rm` | [Bun global cache](https://bun.com/docs/pm/global-cache), [bun pm](https://bun.com/docs/pm/cli/pm) |
| Deno cache | `deno info --json` → `denoDir` | `deno clean` (review: also removes the REPL history kept there) | [deno clean](https://docs.deno.com/runtime/reference/cli/clean/) |
| Corepack | `COREPACK_HOME`, `~/.cache/node/corepack` | `corepack cache clean` | [corepack](https://github.com/nodejs/corepack) |
| node-gyp headers | `~/Library/Caches/node-gyp/<version>` | `node-gyp remove <version>` via npm's bundled node-gyp | node-gyp `lib/remove.js` |
| Global packages | `npm root -g`, `pnpm ls -g --json`, `~/.bun/install/global` | `npm uninstall -g`, `pnpm remove -g`, `bun remove -g` | [npm uninstall](https://docs.npmjs.com/cli/v12/commands/npm-uninstall), [pnpm remove](https://pnpm.io/cli/remove), [bun remove](https://bun.com/docs/pm/cli/remove) |
| nvm | `$NVM_DIR/versions/node/*` | `nvm uninstall <v>` (via `nvm.sh`) | [nvm](https://github.com/nvm-sh/nvm) |
| fnm | `<fnm dir>/node-versions/*` | `fnm uninstall <v>` | [fnm commands](https://github.com/Schniz/fnm/blob/master/docs/commands.md) |
| mise | `mise ls node --json` | `mise uninstall node@<v>` | [mise uninstall](https://mise.jdx.dev/cli/uninstall.html) |
| n · asdf · nodenv · nvm-windows | per tool | `n rm`, `asdf uninstall nodejs`, `nodenv uninstall`, `nvm uninstall` | each tool's README |
| Homebrew | `brew --cellar node@XX` | `brew uninstall node@XX` (protected while `brew uses --installed` lists dependents) | [Homebrew FAQ](https://docs.brew.sh/FAQ) |
| Volta | `~/.volta/tools/image/node/*` | none: Volta cannot uninstall Node versions (report only) | [Volta source](https://github.com/volta-cli/volta) |
| TypeScript, Electron, electron-builder, Playwright, Cypress, Puppeteer caches | `~/Library/Caches/…`, `~/.cache/puppeteer` | none for the whole folder; moved to the Trash | [Playwright browsers](https://playwright.dev/docs/browsers), [Cypress cache](https://docs.cypress.io/app/references/command-line), [Puppeteer](https://pptr.dev/browsers-api), [electron-builder](https://www.electron.build/docs/tutorials/offline-air-gapped-builds) |

Playwright's, Cypress's and Puppeteer's own commands are not used: run through `npx` from outside a project they act on the wrong installation (Cypress `cache prune` keeps only the version running it; `playwright uninstall` without `--all` uses the invoking install).
