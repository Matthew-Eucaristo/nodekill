#!/usr/bin/env bash
# Builds a synthetic machine for the demo recording: a fake HOME (on macOS a
# small disk image, so "free" shows the image), Node projects with their
# folders, package-manager caches, npx entries, global packages and nvm
# versions, plus stand-in npm/pnpm/yarn/node commands that answer nodekill's
# read-only queries from these fixtures. It ends in a shell whose environment
# holds nothing from the real machine.
# Usage: source demo/setup.sh   (from the repository root)
# Remove: hdiutil detach /tmp/nodekill-demo/home (macOS); rm -rf /tmp/nodekill-demo
set -e

DEMO="$(cd /tmp && pwd -P)/nodekill-demo"
[ -d "$DEMO/home" ] && command -v hdiutil >/dev/null && hdiutil detach -quiet "$DEMO/home" 2>/dev/null
[ -d "$DEMO" ] && chmod -R u+w "$DEMO" && rm -rf "$DEMO"
mkdir -p "$DEMO/bin" "$DEMO/home"
go build -trimpath -o "$DEMO/bin/nodekill" .
if command -v hdiutil >/dev/null; then
  hdiutil create -quiet -size 4g -fs APFS -volname demo -type SPARSE "$DEMO/home.sparseimage"
  hdiutil attach -quiet -nobrowse -mountpoint "$DEMO/home" "$DEMO/home.sparseimage"
fi
H="$DEMO/home"
case "$(uname)" in
  Darwin) CACHES="$H/Library/Caches" PNPM_STORE="$H/Library/pnpm/store/v10" ;;
  *) CACHES="$H/.cache" PNPM_STORE="$H/.local/share/pnpm/store/v10" ;;
esac

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_OPTIONAL_LOCKS=0
export GIT_AUTHOR_NAME=demo GIT_AUTHOR_EMAIL=demo@example.com GIT_COMMITTER_NAME=demo GIT_COMMITTER_EMAIL=demo@example.com
blob() { mkdir -p "$1" && dd if=/dev/urandom of="$1/${3:-blob}" bs=1048576 count="$2" status=none; }
stamp_ago() { local t=$(($(date +%s) - $1 * 86400)); date -d "@$t" +%Y%m%d%H%M 2>/dev/null || date -r "$t" +%Y%m%d%H%M; }
iso_ago() { date -u -d "@$(($(date +%s) - $1 * 86400))" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-"$1"d +%Y-%m-%dT%H:%M:%SZ; }
age() { find "$1" -exec touch -h -t "$(stamp_ago "$2")" {} + 2>/dev/null; }
# project dir days package.json-deps lockfile gitignore
project() {
  local dir="$1" days="$2"
  mkdir -p "$dir/src"
  printf '{"name":"%s","private":true,"dependencies":{%s}}\n' "$(basename "$dir")" "$3" > "$dir/package.json"
  printf '{}\n' > "$dir/$4"
  printf '%b' "$5" > "$dir/.gitignore"
  echo 'export default {}' > "$dir/src/index.js"
  git init -q -b main "$dir"
  git -C "$dir" add -A
  GIT_AUTHOR_DATE=$(iso_ago "$days") GIT_COMMITTER_DATE=$(iso_ago "$days") git -C "$dir" commit -q -m init
}
npm_marker() { mkdir -p "$1/node_modules" && echo '{}' > "$1/node_modules/.package-lock.json"; }

# ── projects ──
P="$H/dev/storefront"
project "$P" 95 '"next":"16.0.0","react":"19.2.0"' package-lock.json 'node_modules/\n.next/\n'
echo 'export default {}' > "$P/next.config.ts"
blob "$P/node_modules/next" 210; npm_marker "$P"
blob "$P/.next/cache/turbopack" 140; blob "$P/.next/dev/cache/turbopack" 60
age "$P" 95

P="$H/dev/docs-site"
project "$P" 41 '"@docusaurus/core":"3.9.0"' yarn.lock 'node_modules/\n.docusaurus/\nbuild/\n'
echo 'module.exports = {}' > "$P/docusaurus.config.js"
blob "$P/node_modules/@docusaurus" 130; mkdir -p "$P/node_modules" && touch "$P/node_modules/.yarn-integrity"
blob "$P/.docusaurus" 12; blob "$P/build" 35
age "$P" 41

P="$H/dev/mobile-app"
project "$P" 23 '"expo":"54.0.0","react-native":"0.81.0"' package-lock.json 'node_modules/\n.expo/\nios/Pods/\nios/build/\nandroid/app/build/\nandroid/.gradle/\n'
mkdir -p "$P/ios" "$P/android" && touch "$P/ios/Podfile" "$P/android/build.gradle"
git -C "$P" add -A && GIT_AUTHOR_DATE=$(iso_ago 23) GIT_COMMITTER_DATE=$(iso_ago 23) git -C "$P" commit -q -m native
blob "$P/node_modules/react-native" 190; npm_marker "$P"
blob "$P/ios/Pods" 120; blob "$P/android/app/build" 80; blob "$P/.expo" 2
age "$P" 23

P="$H/dev/dashboard"
project "$P" 1 '"vite":"7.2.0","react":"19.2.0"' pnpm-lock.yaml 'node_modules/\ndist/\n'
echo 'export default {}' > "$P/vite.config.ts"
# pnpm installs by cloning files from its store: they share disk blocks.
blob "$PNPM_STORE/files/00" 170 index
mkdir -p "$P/node_modules/.pnpm/react@19.2.0/node_modules/react"
cp -c "$PNPM_STORE/files/00/index" "$P/node_modules/.pnpm/react@19.2.0/node_modules/react/index.js" 2>/dev/null ||
  ln "$PNPM_STORE/files/00/index" "$P/node_modules/.pnpm/react@19.2.0/node_modules/react/index.js"
echo 'lockfileVersion: 9' > "$P/node_modules/.modules.yaml"
blob "$P/dist" 4
echo '// today' >> "$P/src/index.js"

P="$H/dev/launch-video"
project "$P" 3 '"remotion":"4.0.370","@remotion/cli":"4.0.370"' package-lock.json 'node_modules/\nout/\n'
blob "$P/node_modules/remotion" 160; npm_marker "$P"; blob "$P/out" 220 launch.mp4
age "$P" 3

# ── machine-wide ──
blob "$H/.npm/_cacache/content-v2" 260
for spec in "a1b2c3d4e5f60718:create-next-app@latest:2" "9f8e7d6c5b4a3921:serve:75"; do
  IFS=: read -r hash pkg days <<< "$spec"
  d="$H/.npm/_npx/$hash"
  mkdir -p "$d/node_modules/${pkg%@*}"
  printf '{"_npx":{"packages":["%s"]}}\n' "$pkg" > "$d/package.json"
  blob "$d/node_modules/${pkg%@*}" 25
  age "$d" "$days"
done
blob "$CACHES/Yarn/v6" 150
blob "$CACHES/ms-playwright/chromium-1200" 180
blob "$CACHES/typescript/5.9" 45
G="$H/.npm-global/lib/node_modules"
for spec in "typescript:5.9.3:120" "vercel:48.2.0:200" "npm:11.7.0:5"; do
  IFS=: read -r name ver days <<< "$spec"
  mkdir -p "$G/$name/bin"
  printf '{"name":"%s","version":"%s","bin":{"%s":"bin/cli.js"}}\n' "$name" "$ver" "$name" > "$G/$name/package.json"
  blob "$G/$name" 12 bin/cli.js
  age "$G/$name" "$days"
done
mkdir -p "$H/.nvm/alias" && echo '#' > "$H/.nvm/nvm.sh" && echo 'v22' > "$H/.nvm/alias/default"
for spec in "v18.20.4:300" "v20.18.0:150" "v22.11.0:1"; do
  IFS=: read -r v days <<< "$spec"
  blob "$H/.nvm/versions/node/$v/bin" 40 node
  age "$H/.nvm/versions/node/$v" "$days"
done
blob "$CACHES/node-gyp/18.20.4/include" 8 node.h; age "$CACHES/node-gyp" 300

# ── stand-in tools: answer queries from the fixtures, change nothing ──
cat > "$DEMO/bin/npm" <<EOF
#!/bin/sh
case "\$*" in
  "config get cache"*) echo "$H/.npm" ;;
  --version*) echo 11.7.0 ;;
  "root -g"*) echo "$G" ;;
  "ls -g"*) echo '{"dependencies":{"typescript":{"version":"5.9.3"},"vercel":{"version":"48.2.0"},"npm":{"version":"11.7.0"}}}' ;;
esac
EOF
cat > "$DEMO/bin/pnpm" <<EOF
#!/bin/sh
case "\$*" in
  "store path"*) echo "$PNPM_STORE" ;;
  --version*) echo 10.20.0 ;;
  "ls -g"*) echo '[]' ;;
esac
EOF
cat > "$DEMO/bin/yarn" <<EOF
#!/bin/sh
case "\$*" in
  --version*) echo 1.22.22 ;;
  "cache dir"*) echo "$CACHES/Yarn/v6" ;;
esac
EOF
printf '#!/bin/sh\necho v22.11.0\n' > "$DEMO/bin/node"
chmod +x "$DEMO/bin/"*

# Self-checks: every path nodekill reports lies in the demo HOME, and
# nothing is live. Liveness reads this machine's process table, so a package
# manager running here would show up in the recording.
ENV=(HOME="$H" PATH="$DEMO/bin:/usr/bin:/bin:/usr/sbin" TERM="${TERM:-xterm-256color}" LANG=en_US.UTF-8 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1)
SCAN=$(env -i "${ENV[@]}" "$DEMO/bin/nodekill" scan --json "$H/dev")
if grep -E '"path": "/' <<< "$SCAN"; then
  echo "demo setup: nodekill reports a path outside the demo; not recording" >&2
  return 1 2>/dev/null || exit 1
fi
if grep -E '"level": "live"' <<< "$SCAN" >/dev/null; then
  echo "demo setup: a package manager or dev server is running on this machine; stop it and record again" >&2
  return 1 2>/dev/null || exit 1
fi
cd "$H"
set +e
exec env -i "${ENV[@]}" PS1='$ ' bash --norc --noprofile
