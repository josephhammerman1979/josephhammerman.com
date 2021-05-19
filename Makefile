REPO?=artifactory.squarespace.net/db/squaremeet
DRONE_REPO?=sqsp/db
PRJ_SRC_PATH:=github.com/sqsp/db
BIN_NAME?=squaremeet
CGO_ENABLED?=0
GOOS?=darwin
VERSION?=development
COMMIT_SHA?=$(shell git rev-parse --short HEAD)
BIN_PATH_LINUX:=dist/bin/linux/amd64
BIN_PATH_DARWIN:=dist/bin/darwin/amd64

allPkgs = $(shell go list ./...)

# Ensure we use Go Modules across this project
export GO111MODULE=on
export GOPROXY=https://artifactory.squarespace.net/api/go/go/|direct
export GOPRIVATE=code.squarespace.net,github.com/sqsp
export GONOPROXY=none

.PHONY: all
all: test static-analysis build archive

.PHONY: static-analysis
static-analysis: lint vet errcheck verify-gofmt

.PHONY: fmt
fmt:
	go fmt -s -w ./...

.PHONY: verify-gofmt
verify-gofmt:
	./scripts/verify-gofmt.sh

.PHONY: env
env: deps

.PHONY: deps
deps:
	go mod vendor

.PHONY: tidy
tidy:
	go mod tidy
	go mod vendor

# attempts to build all deps, and env correctly for what's in go.mod
.PHONY: gomod-clean
gomod-clean: clean env gomod

# attempts to fix go mod
.PHONY: gomod
gomod:
	@echo "Running go mod tidy ; go mod vendor until successful (ctrl-c to stop)"
	@false ; while [[ $$? != 0 ]] ; do \
		go mod tidy && go mod vendor; \
	done;
	@echo "Done."

.PHONY: errcheck
errcheck:
	# usrerror types are all errors, so ignore them so that mutative methods on them aren't checked
	go run github.com/kisielk/errcheck -ignore "os:Close" -ignoretests ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	go run github.com/mgechev/revive -formatter friendly $(allPkgs)

.PHONY: build
build: build-linux build-darwin

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=${CGO_ENABLED} go build -mod=vendor --ldflags '-X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.VersionName=${VERSION} -X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.GitCommitSHA=${COMMIT_SHA}' -o ${BIN_PATH_LINUX}/${BIN_NAME} ./cmd/${BIN_NAME}

.PHONY: build-darwin
build-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=${CGO_ENABLED} go build -mod=vendor --ldflags '-X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.VersionName=${VERSION} -X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.GitCommitSHA=${COMMIT_SHA}' -o ${BIN_PATH_DARWIN}/${BIN_NAME} ./cmd/${BIN_NAME}

.PHONY: install-linux
install-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=${CGO_ENABLED} go install -mod=vendor --ldflags '-X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.VersionName=${VERSION} -X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.GitCommitSHA=${COMMIT_SHA}' ./cmd/${BIN_NAME}

.PHONY: install-darwin
install-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=${CGO_ENABLED} go install -mod=vendor --ldflags '-X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.VersionName=${VERSION} -X ${PRJ_SRC_PATH}/cmd/${BIN_NAME}/main.GitCommitSHA=${COMMIT_SHA}' ./cmd/${BIN_NAME}

.PHONY: archive
archive: build
	mkdir -p dist/archive
	tar -c -z -v -C ${BIN_PATH_LINUX} -f dist/archive/${BIN_NAME}_${COMMIT_SHA}_Linux_x86_64.tar.gz ${BIN_NAME}
	tar -c -z -v -C ${BIN_PATH_DARWIN} -f dist/archive/${BIN_NAME}_${COMMIT_SHA}_Darwin_x86_64.tar.gz ${BIN_NAME}

.PHONY: mocks
mocks:
	echo "No mocks currently enabled..."
	# example mockery usage from kdd
	# go run github.com/vektra/mockery/v2 -all -dir pkg/shims -output pkg/shims/mocks
	# go run github.com/vektra/mockery/v2 -all -dir pkg/plugin/cachetreservation -output pkg/plugin/cachetreservation/mocks
	# go run github.com/vektra/mockery/v2 -all -dir pkg/plugin/watchdeploy -inpkg

.PHONY: test
test:
	go test -cover -mod=vendor ./...

.PHONY: coverage
coverage:
	go test -coverprofile coverage.out ./...
	go tool cover -html=coverage.out
	rm coverage.out

.PHONY: clean
clean:
	rm -rf dist/
	go clean -testcache -modcache

.PHONY: container
container: build-linux
	docker build -t ${REPO}:${COMMIT_SHA} -f Dockerfile .
	docker tag ${REPO}:${COMMIT_SHA} ${REPO}:local

.PHONY: publish
publish:
	@echo "Latest Tag Found: $$(git describe --tags --abbrev=0)"
	@echo "Tagged Commit: $$(git log --oneline -1 `git describe --tags --abbrev=0`)"
	@echo "" # formatting
	@read -r -p "What version would you like to publish? " VERSION; \
	git tag -a "$$VERSION" -m "$$VERSION"
	git push --tags
