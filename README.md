# nodekill

**Reclaim the disk space Node.js leaves behind: safely, and the official way.**

nodekill finds every `node_modules`, framework cache and build folder under your projects, plus the machine-wide Node footprint: package-manager caches and stores, `npx` downloads, global packages and old Node versions. It shows how much each one really frees, explains why it is safe or not, and removes what you pick. Folders go to the Trash. Caches, global packages and Node versions are removed only by their own tool (`npm cache clean`, `pnpm store prune`, `npm uninstall -g`, `nvm uninstall`, …).

It is a single Go binary for macOS (first-class), Linux and Windows (best effort). It needs no Node.js of its own, and it never contacts the network itself.

![nodekill scanning Node projects, caches, npx entries, global packages and nvm versions: it selects the idle safe folders, groups them by framework and idle time, and shows exactly what a dry run would move to the Trash and which official commands it would run](demo/demo.gif)

## Install

```sh
brew install matthew-eucaristo/tap/nodekill
```

Or with Go 1.26+: `go install github.com/Matthew-Eucaristo/nodekill@latest`. Release archives for macOS, Linux and Windows are on the [releases page](https://github.com/Matthew-Eucaristo/nodekill/releases).

## Use

```sh
nodekill                       # interactive; scans ~/dev (else the current folder)
nodekill ~/work ~/side         # other roots
nodekill --list                # plain table, read-only
nodekill --dry-run             # interactive, but nothing changes
nodekill --older-than 30d --yes   # headless: trash the safe artifacts of projects idle 30+ days
nodekill scan --json           # machine-readable, read-only
```

In the TUI, `space` selects, `s` selects the safe items in projects and `a` every safe item. `enter` shows exactly what will happen and asks before doing it. `tab` groups by project, framework or idle time, `o` sorts by size, idle time or name, `/` filters, and `?` explains every colour, column and key. The layout fits terminals down to 60 columns, and `NO_COLOR` turns colour off.

```
$ nodekill --list
nodekill: 27 item(s) in 5 project(s), 2.7 GB on disk, 2.0 GB freeable; 1.4 GB safe to clean  (roots: ~/dev)

~/dev/storefront  npm · Next.js · active 3mo ago  429.9 MB
  220.2 MB    safe  node_modules    art-abb796feca8c
  209.7 MB    safe  .next           art-73f65e785488

~/dev/dashboard  pnpm · Vite · active today  182.5 MB
  178.3 MB  frees 4.1 kB  review  node_modules  recent             art-438c555dd351
  4.2 MB                  review  dist          deployable recent  art-602cb3e21379

Caches and stores  852.5 MB
  272.6 MB             safe    npm cache                          via `npm cache clean --force`  glb-8b26737c8a2b
  178.3 MB  frees 0 B  safe    pnpm store                         via `pnpm store prune`         glb-3bc1271be3b4
  157.3 MB             safe    Yarn 1 cache                       via `yarn cache clean`         glb-5c41ddcb5249
…
Node versions  125.8 MB
  41.9 MB    protected  Node v22.11.0 (nvm)  current via `nvm uninstall v22.11.0`         glb-52981247f837
  41.9 MB    review     Node v20.18.0 (nvm)  catalog_review via `nvm uninstall v20.18.0`  glb-c42b7e64b486
  41.9 MB    review     Node v18.20.4 (nvm)  catalog_review via `nvm uninstall v18.20.4`  glb-f6aa4093db0f
```

(The demo workspace from `demo/setup.sh`, trimmed. The `dashboard` dependencies are clones of the pnpm store, so removing them frees almost nothing until the store is pruned.)

At the end nodekill prints a receipt that keeps three numbers apart, because they mean different things:

```
nodekill receipt
  ✓ Moved to Trash       3.1 GB  4 item(s); restore from the Trash
  ✓ Freed by tools     852.5 MB  2 official command(s)
  Free space changed by +852.5 MB (measured); another ~3.1 GB when you empty the Trash
```

## What it finds

**In projects** (a folder with `package.json`). A folder counts only at its exact place next to the file that proves the tool is used, never by name alone. Test runners and build tools hoisted to a workspace root count for its members.

| Folder | When | Level |
|---|---|---|
| `node_modules` | always | safe |
| `.next` (with Turbopack's `.next/cache` and `.next/dev/cache`) | `next` or `next.config.*` | safe; review with `output: 'standalone'` |
| `.nuxt`, `.nitro` · `.output` | Nuxt, Nitro, SolidStart, TanStack Start | safe · review |
| `.svelte-kit`, `.angular/cache`, `.turbo/cache`, `.nx/cache`, `.nx/workspace-data`, `.moon/cache`, `.parcel-cache`, `.expo`, `.docusaurus`, `.vitepress/cache`, `.react-router`, `.vinxi`, `.tanstack/tmp`, `.cache` (Gatsby), `.firebase`, `.yarn/unplugged`, `.nyc_output` | their tool's dependency or config | safe |
| `.wrangler/tmp`, `.wrangler/cache`, `.netlify/functions-serve`, `.sst/platform`, `.sst/log`, `cdk.out`, `.mastra/.build` | Wrangler, Netlify, SST, AWS CDK, Mastra; their local data and state are never touched | safe |
| Playwright `test-results`, `blob-report` · `playwright-report`; Cypress `videos`, `screenshots`, `downloads` | the test runner | safe · review |
| React Native and Capacitor: `ios/Pods` · `ios/build`, `vendor/bundle`, `android/app/build`, `android/app/.cxx`, `android/build`, `android/.gradle` | `react-native`/`expo`/`@capacitor/*` with a Podfile, Gemfile or Gradle build | review · safe |
| `src-tauri/target` (Tauri), `.astro`, `.open-next`, `.vercel/output`, `.serverless`, `.medusa/server`, `.strapi` | their tool | review |
| `dist`, `build`, `out`, `_site`, `coverage`, `storybook-static`, Hardhat `artifacts` | a builder that writes them (Vite, Astro, Rsbuild, Rspack, tsdown, Docusaurus, Next static export, Electron Forge, electron-vite, Eleventy, Hardhat, …) | review; protected outside git |
| Hardhat `cache` | Hardhat (not Foundry) | safe; protected outside git |
| Remotion `out` | Remotion | protected: these are your renders |

**On the machine**, each removed only by its owner's command, which nodekill runs from your home folder:

| Item | Removed with | What the tool itself advises |
|---|---|---|
| npm cache | `npm cache clean --force` | npm calls the cache self-healing; use `npm cache verify` for problems, clean only to reclaim space |
| each `npx` download, with when it last ran | `npm cache npx rm <key>` (npm 11.2+) | npx downloads it again when needed |
| pnpm store | `pnpm store prune` (only packages no project references) | safe, but not too often: pruned packages download again |
| Yarn 1 cache · Yarn 2+ global cache | `yarn cache clean` (Yarn 2+ only when its `cacheFolder` is that folder) | Zero-Installs projects keep their own `.yarn/cache` |
| Bun cache | `bun pm cache rm --cache-dir <dir>` | `bun pm cache prune` is the gentler option |
| Deno cache | `deno clean` | it removes all of DENO_DIR, including the REPL history: review. Protected when it holds localStorage/KV data or Deno's install folder |
| Corepack cache | `corepack cache clean` (Corepack 0.25+) | |
| node-gyp headers, per Node version · nvm's download cache | `node-gyp remove <version>` (npm's bundled copy) · `nvm cache clear` | |
| global packages (npm, pnpm, Bun, Yarn 1, `deno install`), with when each last ran | `npm uninstall -g` · `pnpm remove -g` · `bun remove -g` · `yarn global remove` · `deno uninstall -g` | npm and corepack are never offered |
| Node versions from nvm, fnm, mise, n, asdf, nodenv, Homebrew `node@XX` | the manager's own uninstall (`brew uninstall` without autoremove) | the version on your PATH and each manager's default are protected |
| Node versions from Volta, nvm-windows, nvs, proto, nvm.fish | listed with the command for you to run | Volta cannot uninstall; the others are not run until their behaviour is verified |
| TypeScript typings, Electron, electron-builder, Playwright, Cypress and Puppeteer downloads | no command exists: moved to the Trash | |

If the owning tool is not installed (a Yarn 2+ cache with no Yarn 2+ on `PATH`, say), the item is shown with the command to run, and nodekill leaves it alone. Outside a project, Corepack's default `yarn` is Yarn 1, so the Yarn 2+ cache is often in that state. Old pnpm store formats (`store/v10` beside `v11`) have no pnpm command and move to the Trash after review. [docs/rules.md](docs/rules.md) links the source for every rule and command.

## Safety model

nodekill refuses anything it cannot verify. Every item gets a level and the evidence behind it:

- **safe**: rebuildable, git-ignored with no tracked files, project idle 7+ days, nothing running in it. These may be bulk-selected (empty folders are skipped).
- **review**: allowed, never bulk-selected. Reasons include:
  - the project changed this week;
  - there is no lockfile, so a reinstall may pick newer versions;
  - files inside `node_modules` changed after the install (local patches);
  - the output may be what you deploy;
  - it sits in a linked git worktree that an agent may still use (fell manages the worktree itself);
  - some files inside could not be read;
  - it is a global tool or Node version you installed on purpose.
- **protected**: shown, never removed. Reasons include:
  - git tracks files inside, or git does not ignore it;
  - it is a generic name such as `dist` outside a repository;
  - it holds your data, or another repository lies inside;
  - its tool is missing;
  - the tool's cleanup would empty a folder outside your home or a top-level folder of it;
  - it is the Node you are running, a manager's default, or `npm`/`corepack` themselves.
- **live**: a dev server, build, test runner or the package manager is working in it right now.

Right before acting, nodekill reads the item again and refuses it if anything safety-relevant changed since the scan. It also re-checks that the path is a real directory at its catalog position; symlinks are never followed or removed through. Commands run with a fixed argument list, never through a shell. The one exception is nvm, which is a shell function: nodekill sources its own `nvm.sh` and passes the version as an argument. `--dry-run`, `--list`, `--json` and `scan` never remove anything. To find caches, nodekill asks your tools read-only questions such as `npm config get cache` and `pnpm store path`. It turns off their prompts, update checks and downloads while it does. A tool may still touch its own folders when asked, for example pnpm updating its home folder's timestamp.

The Trash is the default. `--permanent` deletes folders instead and turns the header red. Official commands delete by their tool's own design, so the confirmation screen lists them apart.

## How sizes are counted

Sizes are allocated blocks, like `du`. A hard-linked file counts once. **FREES** appears when much of an item is shared with something outside it, and shows what removing it frees right now:

- pnpm and Bun install from a global store. On macOS they **clone** files (APFS copy-on-write), and on Linux pnpm hard-links them. A pnpm `node_modules` that `du` calls 567 MB may free 30 MB until `pnpm store prune` runs.
- Folders cloned by agent tools or Finder share blocks the same way.

On macOS nodekill asks APFS for each file's private size (`ATTR_CMNEXT_PRIVATESIZE`). Btrfs and XFS reflinks on Linux are not visible and count in full. After acting, freed space is measured from the disk, never assumed. Moving to the Trash frees nothing until you empty it, and the receipt says so.

## Automation and the JSON contract

`nodekill scan --json` (or `--ndjson` to stream) prints every item with its id, fingerprint, sizes, evidence and the exact command it would run. Paths are home-relative. `nodekill apply --plan <file|->` takes ids and fingerprints back, re-checks each item and acts. `nodekill capabilities --json` lists actions, reason codes and exit codes. The format is shared with the other tools in this family and described in [docs/contract.md](docs/contract.md).

Exit codes: 0 ok · 1 error · 2 usage · 3 something was refused · 4 something failed.

## Platforms

- **macOS**: first-class. Sizes account for APFS clones. The Trash supports Put Back (`/usr/bin/trash` on macOS 15+, else Finder). Running processes are checked with `lsof`.
- **Linux**: freedesktop Trash, `/proc` process checks, hard-link-aware sizes.
- **Windows**: best effort, with the full test suite running in CI. Folders go to the Recycle Bin; folders whose paths would grow past the 260-character limit inside it are refused, so use `--permanent` for those. There is no running-process check, so nothing is bulk-selected. Sizes are measured without hard-link detection.

## Demo

`demo/demo.tape` records `demo/demo.gif` with [VHS](https://github.com/charmbracelet/vhs) against generated fixtures: a fake home on a small disk image, plus stand-in `npm`, `pnpm`, `yarn` and `node` commands. Nothing from the real machine appears. To try the same workspace yourself, run `source demo/setup.sh`, then `nodekill`.

## Uninstall

nodekill writes no configuration, cache, log or state anywhere. The only things it changes are the items you confirm. On Linux, the first move to the Trash creates the standard `~/.local/share/Trash` folders if they are missing.

```sh
brew uninstall nodekill
brew autoremove                     # removes Go if it was installed only to build nodekill
brew cleanup --prune=all            # optional: clears Homebrew's download and build cache
brew untap matthew-eucaristo/tap    # only if nothing else is installed from this tap
```

Installed with `go install`? Run `rm "$(go env GOPATH)/bin/nodekill"`. Anything nodekill moved to the Trash stays there until you empty it or put it back.

## Credits

The catalog was re-derived from each tool's documentation and source; [docs/rules.md](docs/rules.md) links them. Prior art studied: [npkill](https://github.com/voidcosmos/npkill) (MIT), whose interaction model inspired the TUI; [kondo](https://github.com/tbillington/kondo) (MIT) and [gdu](https://github.com/dundee/gdu) (MIT) for marker-based matching; [Mole](https://github.com/tw93/Mole) (GPL-3.0, used only as a checklist, no code). nodekill shares its safety model and JSON contract with [fell](https://github.com/Matthew-Eucaristo/fell) and [flutterkill](https://github.com/Matthew-Eucaristo/flutterkill).

## License

[MIT](LICENSE)
