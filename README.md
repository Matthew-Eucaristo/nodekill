# nodekill

**Reclaim the disk space Node.js leaves behind: safely, and the official way.**

nodekill finds every `node_modules`, framework cache and build folder under your projects, plus the machine-wide Node footprint: package-manager caches and stores, `npx` downloads, global packages and old Node versions. It shows how much each one really frees, explains why it is safe or not, and removes what you pick. Folders go to the Trash. Caches, global packages and Node versions are removed only by their own tool (`npm cache clean`, `pnpm store prune`, `npm uninstall -g`, `nvm uninstall`, …).

It is a single Go binary for macOS (first-class), Linux and Windows (best effort). It needs no Node.js of its own and makes no network requests.

```
 nodekill  ● trash  ·  26.7 GB on disk  ·  22.8 GB freeable  ·  8.7 GB safe  ·  211.4 GB free  ·  ~/dev

                         SIZE      FREES  IDLE      SAFETY      ITEM  (group: project · sort: size)
  ○ ~/dev/old-shop  1.6 GB  npm · Next.js · active 3mo ago
  ● █████▍········   831.2 MB             3mo       ● safe      node_modules
  ● ████▊·········   725.3 MB             3mo       ● safe      .next
  ○ ~/dev/dashboard  1.5 GB  npm · Vite · active today
  ○ █████████▊····     1.5 GB             today     ◐ review    node_modules  recent
  ○ ~/dev/video  2.0 GB  npm · Remotion · active 3d ago
    ████████······     1.2 GB             3d        ⊘ protected out  your data
  ○ ~/dev/mono  567.0 MB  pnpm · TanStack, Vite · active 2w ago
  ○ ███▋··········   567.0 MB    30.0 MB  2w        ● safe      node_modules
  ○ Caches and stores  2.8 GB  6 item(s)
  ○ ██████████████     2.1 GB   784.9 MB  —         ● safe      pnpm store  via pnpm store prune
    ███▍··········   513.2 MB             —         ⊘ protected Yarn 2+ global cache  not installed
  ○ Global packages and npx  63.6 MB  2 item(s)
  ○ ▎·············    46.4 MB             8mo       ◐ review    typescript@5.9.3  check first via npm uninstall -g typescript
  ○ Node versions  262.1 MB  2 item(s)
  ○ █▋············   181.0 MB             5mo       ◐ review    Node v20.19.5 (nvm)  check first via nvm uninstall v20.19.5
 ───────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ~/dev/old-shop/node_modules  831.2 MB on disk
  Installed dependencies. Your package manager recreates them from the lockfile.
  Action: move to the Trash  ·  comes back with: npm ci
  rebuildable, ignored by git, not in use
  2 selected (1.6 GB)  ·  moved to Trash 3.1 GB  ·  free space +0 B measured
  ↑↓ move  space select  s safe in projects  a all safe  enter clean  / filter  tab group  o sort  ? legend  q quit
```

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

In the TUI, `space` selects, `s` selects the safe items in projects, `a` every safe item, `enter` shows exactly what will happen and asks before doing it. `tab` groups by project, framework or idle time, `o` sorts by size, idle time or name, `/` filters, and `?` explains every colour and column.

At the end nodekill prints a receipt that keeps three numbers apart, because they mean different things:

```
nodekill receipt
  ✓ Moved to Trash       3.1 GB  4 item(s); restore from the Trash
  ✓ Freed by tools     852.5 MB  2 official command(s)
  Free space changed by +852.5 MB (measured); another ~3.1 GB when you empty the Trash
```

## What it finds

**In projects** (a folder with `package.json`). A folder counts only at its exact place next to the file that proves the tool is used, never by name alone.

| Folder | When | Level |
|---|---|---|
| `node_modules` | always | safe |
| `.next` (incl. Turbopack's `.next/cache` and `.next/dev/cache`) | `next` dependency or `next.config.*` | safe; review with `output: 'standalone'` |
| `.nuxt` · `.output` | Nuxt / Nitro | safe · review |
| `.svelte-kit`, `.angular/cache`, `.turbo`, `.nx/cache`, `.nx/workspace-data`, `.parcel-cache`, `.expo`, `.docusaurus`, `.react-router`, `.vinxi`, `.tanstack/tmp`, `.cache` (Gatsby), `.firebase`, `.yarn/unplugged`, `.nyc_output` | their tool's dependency or config | safe |
| `.wrangler/tmp`, `.wrangler/cache` | Wrangler (local D1/KV/R2 data in `.wrangler/state` is never touched) | safe |
| `.vercel/output` | Vercel (the project link and pulled env files are never touched) | review |
| React Native: `ios/Pods` · `ios/build`, `android/app/build`, `android/.gradle` | `react-native`/`expo` with a Podfile or Gradle build | review · safe |
| `dist`, `build`, `out`, `coverage`, `storybook-static` | a builder that writes them (Vite, Astro, Docusaurus, Next static export, …) | review; protected outside git |
| Remotion `out` | Remotion | protected: these are your renders |

**On the machine**, each removed only by its owner's command:

| Item | Removed with |
|---|---|
| npm cache | `npm cache clean --force` |
| each `npx` download, with when it last ran | `npm cache npx rm <key>` |
| pnpm store | `pnpm store prune` (only packages no project references) |
| Yarn 1 cache · Yarn 2+ global cache | `yarn cache clean` · `yarn cache clean --all` |
| Bun cache · Deno cache · Corepack cache | `bun pm cache rm` · `deno clean` · `corepack cache clean` |
| node-gyp headers, per Node version | `node-gyp remove <version>` (npm's bundled copy) |
| global packages (npm, pnpm, Bun), with when each last ran | `npm uninstall -g` · `pnpm remove -g` · `bun remove -g` |
| Node versions from nvm, fnm, mise, n, asdf, nodenv, nvm-windows, Homebrew `node@XX` | the manager's own uninstall |
| TypeScript typings, Electron, electron-builder, Playwright, Cypress and Puppeteer downloads | no command exists: moved to the Trash |

If the owning tool is not installed (a Yarn 2+ cache with no Yarn on `PATH`, say), the item is shown with the command to run, and nodekill leaves it alone. Old pnpm store formats (`store/v10` beside `v11`) have no pnpm command and move to the Trash after review. Volta cannot uninstall Node versions, so its versions are listed for information only.

## Safety model

nodekill refuses anything it cannot verify. Every item gets a level and the evidence behind it:

- **safe**: rebuildable, git-ignored with no tracked files, project idle 7+ days, nothing running in it. These may be bulk-selected.
- **review**: allowed, never bulk-selected. The project changed this week, there is no lockfile (a reinstall may pick newer versions), files inside `node_modules` changed after the install (local patches), the output may be what you deploy, or it is a global tool or Node version you installed on purpose.
- **protected**: shown, never removed. Git tracks files inside, or git does not ignore it. It is a generic name such as `dist` outside a repository. It holds your data. Another repository lies inside. Its tool is missing. It is the Node you are running, a manager's default, or `npm`/`corepack` themselves.
- **live**: a dev server, build, test runner or the package manager is working in it right now.

Right before acting, nodekill reads the item again, refuses it if anything safety-relevant changed since the scan, and re-checks that the path is a real directory at its catalog position (symlinks are never followed or removed through). Commands run with a fixed argument list, never through a shell. `--dry-run`, `--list`, `--json` and `scan` never change anything.

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

- **macOS**: first-class. APFS clone-aware sizes, Trash with Put Back (`/usr/bin/trash` on macOS 15+, else Finder), running-process checks via `lsof`.
- **Linux**: freedesktop Trash, `/proc` process checks, hard-link-aware sizes.
- **Windows**: best effort. Recycle Bin (folders whose paths would grow past the 260-character limit inside it are refused; use `--permanent`), no running-process check (so nothing is bulk-selected), sizes without hard-link detection.

## Uninstall

```sh
brew uninstall nodekill
brew untap matthew-eucaristo/tap   # if nothing else uses it
```

nodekill writes no configuration, cache, log or state anywhere. The only things it changes are the items you confirm.

## Credits

The catalog was re-derived from each tool's documentation and source; [docs/rules.md](docs/rules.md) links them. Prior art studied: [npkill](https://github.com/voidcosmos/npkill) (MIT), whose interaction model inspired the TUI; [kondo](https://github.com/tbillington/kondo) (MIT) and [gdu](https://github.com/dundee/gdu) (MIT) for marker-based matching; [Mole](https://github.com/tw93/Mole) (GPL-3.0, used only as a checklist, no code). nodekill shares its safety model and JSON contract with [fell](https://github.com/Matthew-Eucaristo/fell) and [flutterkill](https://github.com/Matthew-Eucaristo/flutterkill).

## License

[MIT](LICENSE)
