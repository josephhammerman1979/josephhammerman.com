#!/usr/bin/env bash
# build_wasm.sh – compile the dice game to WebAssembly and copy support files.
#
# Run from the repo root:
#   ./build_wasm.sh
#
# Requirements: Go toolchain with WASM support (go 1.21+).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
WASM_OUT_DIR="$REPO_ROOT/app/wasm"
JS_OUT_DIR="$REPO_ROOT/app/controllers/js"
DICE_DIR="$REPO_ROOT/dicegames"

echo "==> Building pig.wasm …"
(
  cd "$DICE_DIR"
  GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o "$WASM_OUT_DIR/pig.wasm" .
)
echo "    pig.wasm -> $WASM_OUT_DIR/pig.wasm"

echo "==> Copying wasm_exec.js from Go toolchain …"
GOROOT="$(go env GOROOT)"
WASM_EXEC_SRC=""
for cand in "$GOROOT/lib/wasm/wasm_exec.js" "$GOROOT/misc/wasm/wasm_exec.js"; do
  if [ -f "$cand" ]; then WASM_EXEC_SRC="$cand"; break; fi
done
if [ -z "$WASM_EXEC_SRC" ]; then
  # Some distro packages (e.g. Debian/Ubuntu golang-1.24-go) ship the binaries
  # but not the wasm support files; search the full GOROOT as a fallback.
  WASM_EXEC_SRC="$(find "$GOROOT" -type f -name wasm_exec.js 2>/dev/null | head -n1)"
fi
if [ -z "$WASM_EXEC_SRC" ] || [ ! -f "$WASM_EXEC_SRC" ]; then
  echo "ERROR: cannot find wasm_exec.js under $GOROOT" >&2
  echo "       On Debian/Ubuntu, install the matching source package, e.g.:" >&2
  echo "           sudo apt-get install golang-1.24-src" >&2
  exit 1
fi
cp "$WASM_EXEC_SRC" "$JS_OUT_DIR/wasm_exec.js"
echo "    wasm_exec.js -> $JS_OUT_DIR/wasm_exec.js"

echo "==> Done. Serve the app and navigate to a /rooms/<id> room to play."
