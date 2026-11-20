REPO_ROOT  := $(CURDIR)
SERVER_BIN := $(REPO_ROOT)/josephhammerman.com
# Files embedded into the server binary via //go:embed — changes to any of
# these must trigger a rebuild even though they are not .go sources.
EMBED_SRC  := $(shell find $(REPO_ROOT)/app/controllers/templates $(REPO_ROOT)/app/controllers/css $(REPO_ROOT)/app/controllers/js -type f 2>/dev/null)

.PHONY: all build run test clean

all: build

build: $(SERVER_BIN)

$(SERVER_BIN): $(shell find $(REPO_ROOT)/app -type f -name '*.go') $(REPO_ROOT)/server.go $(EMBED_SRC)
	@echo "==> Building server"
	go build -o $@ .

run: build
	@echo "==> Starting server on :8000"
	$(SERVER_BIN)

test:
	go test ./...

clean:
	rm -f $(SERVER_BIN)
