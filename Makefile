REPO_ROOT  := $(CURDIR)
DICE_DIR   := $(REPO_ROOT)/dicegames
WASM_OUT   := $(REPO_ROOT)/app/wasm/pig.wasm
JS_OUT_DIR := $(REPO_ROOT)/app/controllers/js
WASM_EXEC  := $(JS_OUT_DIR)/wasm_exec.js
WASM_EXEC_SRC := $(REPO_ROOT)/third_party/wasm/wasm_exec.js
SERVER_BIN := $(REPO_ROOT)/josephhammerman.com
DICE_SRC   := $(shell find $(DICE_DIR) -type f -name '*.go')
# Files embedded into the server binary via //go:embed — changes to any of
# these must trigger a rebuild even though they are not .go sources.
EMBED_SRC  := $(shell find $(REPO_ROOT)/app/controllers/templates $(REPO_ROOT)/app/controllers/css $(REPO_ROOT)/app/controllers/js -type f 2>/dev/null)

.PHONY: all wasm build run clean vendor-wasm-exec

all: build

wasm: $(WASM_OUT) $(WASM_EXEC)

$(WASM_OUT): $(DICE_SRC)
	@mkdir -p $(dir $@)
	@echo "==> Building pig.wasm"
	cd $(DICE_DIR) && GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o $@ .

$(WASM_EXEC): $(WASM_EXEC_SRC)
	@mkdir -p $(JS_OUT_DIR)
	@echo "==> Copying vendored wasm_exec.js"
	@cp "$(WASM_EXEC_SRC)" "$@"

build: wasm $(SERVER_BIN)

$(SERVER_BIN): $(shell find $(REPO_ROOT)/app -type f -name '*.go') $(REPO_ROOT)/server.go $(EMBED_SRC)
	@echo "==> Building server"
	go build -o $@ .

run: build
	@echo "==> Starting server on :8000"
	$(SERVER_BIN)

clean:
	rm -f $(WASM_OUT) $(WASM_EXEC) $(SERVER_BIN)

# Refresh the vendored wasm_exec.js from the local Go toolchain.
# Run this after upgrading Go (e.g. 1.24 -> 1.25), then commit the result.
vendor-wasm-exec:
	@goroot=$$(go env GOROOT); \
	src="$$goroot/lib/wasm/wasm_exec.js"; \
	if [ ! -f "$$src" ]; then src="$$goroot/misc/wasm/wasm_exec.js"; fi; \
	if [ ! -f "$$src" ]; then \
	  src=$$(find "$$goroot" -type f -name wasm_exec.js 2>/dev/null | head -n1); \
	fi; \
	if [ -z "$$src" ] || [ ! -f "$$src" ]; then \
	  echo "ERROR: cannot find wasm_exec.js under $$goroot." >&2; \
	  echo "       Install the Go source package for your distro and retry." >&2; \
	  exit 1; \
	fi; \
	mkdir -p $(dir $(WASM_EXEC_SRC)); \
	if cmp -s "$$src" "$(WASM_EXEC_SRC)"; then \
	  echo "==> $(WASM_EXEC_SRC) already up to date ($$(go version))"; \
	else \
	  cp "$$src" "$(WASM_EXEC_SRC)"; \
	  echo "==> Updated $(WASM_EXEC_SRC) from $$src ($$(go version))"; \
	  echo "    Don't forget to commit the change."; \
	fi
