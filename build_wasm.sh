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
WASM_EXEC_SRC="$GOROOT/misc/wasm/wasm_exec.js"
if [ ! -f "$WASM_EXEC_SRC" ]; then
  # Go 1.24+ moved it
  WASM_EXEC_SRC="$GOROOT/lib/wasm/wasm_exec.js"
fi
if [ ! -f "$WASM_EXEC_SRC" ]; then
  echo "ERROR: cannot find wasm_exec.js under $GOROOT" >&2
  exit 1
fi
cp "$WASM_EXEC_SRC" "$JS_OUT_DIR/wasm_exec.js"
echo "    wasm_exec.js -> $JS_OUT_DIR/wasm_exec.js"

echo "==> Done. Serve the app and navigate to a /rooms/<id> room to play."
