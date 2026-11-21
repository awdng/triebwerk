# runtime options
COMMIT_HASH = $(shell git rev-parse --short HEAD)
TAG         = $(shell git symbolic-ref -q --short HEAD || git describe --tags --exact-match)
# -e: cmd/wasm only builds for js/wasm, without it go list prints a build constraint error
GOPACKAGES  = $(shell go list -e ./... | grep -v cmd/wasm)
GOFILES		= $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./cmd/wasm/*")
HAS_GOLINT  = $(shell command -v golint)

# go options
GO       ?= go
# pinned in go.mod (tool directive), nothing to install
GOIMPORTS = $(GO) tool goimports
# the race detector needs cgo, and cgo needs a C compiler (gcc)
GOTEST   = CGO_ENABLED=1 $(GO) test -race
LDFLAGS  = -X "main.CommitHash=$(COMMIT_HASH)" -X "main.Tag=$(TAG)"

help: ## Display this help
	@ echo "Please use \`make <target>' where <target> is one of:"
	@ echo
	@ grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[36m%-10s\033[0m - %s\n", $$1, $$2}'
	@ echo

all: build ## Install dependencies and build binaries

run: build
	@export `cat ${mkfile_path}.env | xargs`; ./triebwerk

tools: ## Install all necessary tools
	$(GO) install golang.org/x/lint/golint@latest

fmt-check: ## Check formatting (goimports), fails if a file needs formatting
	@ out=$$($(GOIMPORTS) -l $(GOFILES)) || exit 1; \
	  test -z "$$out" || ($(GOIMPORTS) -d $(GOFILES); echo "run make fmt"; exit 1)

fmt: ## Fix formatting (goimports)
	@ $(GOIMPORTS) -w $(GOFILES)

lint: ## Perform lint checks
ifndef HAS_GOLINT
	$(GO) install golang.org/x/lint/golint@latest
endif
	golint -set_exit_status $(GOPACKAGES)

vet: ## Perform vet checks
	go vet $(GOPACKAGES)

test: fmt-check vet test-unit ## Execute all checks and tests, includes the 12 player load test (about 40s)

test-unit: ## Execute unit tests and the in-process 12 player load test
	$(GOTEST) -v $(GOPACKAGES)

test-short: ## Execute unit tests without the load test
	$(GOTEST) -short $(GOPACKAGES)

# The race detector needs a C compiler. A machine without one that has
# Docker can run the tests in a Go image; modules and the pinned toolchain
# come from this machine's module cache.
test-docker: ## The race tests of `make test` in a Go container, for machines without a C compiler
	./scripts/test-docker.sh

wasm: ## Build the client simulation (cmd/wasm) into the client
	GOOS=js GOARCH=wasm $(GO) build -o ../client/public/game/tanks.wasm ./cmd/wasm

integration-test: ## Execute integration tests
	@test -f .env || (echo "File \".env\" does not exist and is needed to run integration test" && exit 1)
	@export `cat ${mkfile_path}.env | xargs`; $(GOTEST) -v -tags integration ./...

cover: ## Execute unit tests with coverage
	$(GOTEST) -v -covermode=atomic -coverpkg=$(shell echo $(GOPACKAGES) | tr " " ",") -coverprofile=cover.out ./...

cover-html: cover ## Generate and show HTML coverage report
	$(GO) tool cover -html=cover.out

build: fmt ## Build binaries
	$(GO) build -ldflags '$(LDFLAGS)' -o ./triebwerk cmd/server/main.go

build-static: fmt ## Build binaries statically
	CGO_ENABLED=0 $(GO) build -ldflags '$(LDFLAGS)' -v -a -installsuffix cgo -o ./triebwerk cmd/server/main.go

.env:
	@test -f $(MKFILE_PATH).env || (echo "File \".env\" does not exist and is needed to run integration test" && exit 1)
	@export `cat $(MKFILE_PATH).env | xargs`;

loadtest: ## Run a local load test with a live view on http://localhost:8090
	./scripts/loadtest.sh

qa: ## Server, bots and the ../client build for other machines (Tailscale), see scripts/qa.sh
	./scripts/qa.sh

# own ports, so a running `make loadtest` or local game does not collide
loadtest-check: ## Run the real binaries with 12 players for one round, fail on drops, stalls or missing rounds
	MASTER_PORT=28081 GAME_PORT=29090 GAME_LENGTH=15 ./scripts/loadtest.sh -n 12 -d 35s -check -view ''

loadtest-check-webtransport: ## The same with the players connected over WebTransport
	TRANSPORT=webtransport MASTER_PORT=28081 GAME_PORT=29090 GAME_LENGTH=15 ./scripts/loadtest.sh -n 12 -d 35s -check -view ''

clean: ## Cleanup runtime files
	rm -rf triebwerk *.out

clean-all: clean ## Cleanup ALL runtime files
	rm -rf triebwerk

.PHONY: help all run tools deps fmt-check fmt lint vet test test-unit test-short test-docker wasm integration-test cover cover-html build build-static loadtest loadtest-check loadtest-check-webtransport qa clean clean-all
