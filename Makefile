REPO_ROOT  := $(CURDIR)
DICE_DIR   := $(REPO_ROOT)/dicegames
WASM_OUT   := $(REPO_ROOT)/app/wasm/pig.wasm
JS_OUT_DIR := $(REPO_ROOT)/app/controllers/js
WASM_EXEC  := $(JS_OUT_DIR)/wasm_exec.js
SERVER_BIN := $(REPO_ROOT)/josephhammerman.com
GOROOT     := $(shell go env GOROOT)
DICE_SRC   := $(shell find $(DICE_DIR) -type f -name '*.go')

.PHONY: all wasm build run clean

all: build

wasm: $(WASM_OUT) $(WASM_EXEC)

$(WASM_OUT): $(DICE_SRC)
	@mkdir -p $(dir $@)
	@echo "==> Building pig.wasm"
	cd $(DICE_DIR) && GOOS=js GOARCH=wasm go build -ldflags="-s -w" -o $@ .

$(WASM_EXEC):
	@mkdir -p $(JS_OUT_DIR)
	@echo "==> Locating wasm_exec.js under $(GOROOT)"
	@src=""; \
	for cand in "$(GOROOT)/lib/wasm/wasm_exec.js" "$(GOROOT)/misc/wasm/wasm_exec.js"; do \
	  if [ -f "$$cand" ]; then src="$$cand"; break; fi; \
	done; \
	if [ -z "$$src" ]; then \
	  src=$$(find "$(GOROOT)" -type f -name wasm_exec.js 2>/dev/null | head -n1); \
	fi; \
	if [ -z "$$src" ]; then \
	  echo "ERROR: cannot find wasm_exec.js under $(GOROOT)." >&2; \
	  echo "       On Debian/Ubuntu, install the matching source package, e.g.:" >&2; \
	  echo "           sudo apt-get install golang-1.24-src" >&2; \
	  exit 1; \
	fi; \
	cp "$$src" "$@"; \
	echo "    $$src -> $@"

build: wasm $(SERVER_BIN)

$(SERVER_BIN): $(shell find $(REPO_ROOT)/app -type f -name '*.go') $(REPO_ROOT)/server.go
	@echo "==> Building server"
	go build -o $@ .

run: build
	@echo "==> Starting server on :8000"
	$(SERVER_BIN)

clean:
	rm -f $(WASM_OUT) $(WASM_EXEC) $(SERVER_BIN)
