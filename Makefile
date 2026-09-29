# runtime options
COMMIT_HASH = $(shell git rev-parse --short HEAD)
TAG         = $(shell git symbolic-ref -q --short HEAD || git describe --tags --exact-match)
GOPACKAGES  = $(shell go list ./... | grep -v cmd/wasm)
GOFILES		= $(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./cmd/wasm/*")
HAS_GOLINT  = $(shell command -v golint)

# go options
GO       ?= go
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
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install golang.org/x/lint/golint@latest

fmt-check: ## Check formatting (goimports), fails if a file needs formatting
	@ test -z "$$(goimports -l $(GOFILES))" || (goimports -d $(GOFILES); echo "run make fmt"; exit 1)

fmt: ## Fix formatting (goimports)
	@ goimports -w $(GOFILES)

lint: ## Perform lint checks
ifndef HAS_GOLINT
	$(GO) install golang.org/x/lint/golint@latest
endif
	golint -set_exit_status $(GOPACKAGES)

vet: ## Perform vet checks
	go vet $(GOPACKAGES)

test: fmt-check vet test-unit ## Execute all checks and tests, includes the 12 player load test (about 40s)

test-unit: ## Execute unit tests and the in-process 12 player load test
	$(GO) test -race -v $(GOPACKAGES)

test-short: ## Execute unit tests without the load test
	$(GO) test -race -short $(GOPACKAGES)

wasm: ## Build the client simulation (cmd/wasm) into the client
	GOOS=js GOARCH=wasm $(GO) build -o ../client/public/game/tanks.wasm ./cmd/wasm

integration-test: ## Execute integration tests
	@test -f .env || (echo "File \".env\" does not exist and is needed to run integration test" && exit 1)
	@export `cat ${mkfile_path}.env | xargs`; $(GO) test -v -race -tags integration ./...

cover: ## Execute unit tests with coverage
	$(GO) test -v -race -covermode=atomic -coverpkg=$(shell echo $(GOPACKAGES) | tr " " ",") -coverprofile=cover.out ./...

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

loadtest-check: ## Run the real binaries with 12 players for one round, fail on drops, stalls or missing rounds
	GAME_LENGTH=15 ./scripts/loadtest.sh -n 12 -d 35s -check -view ''

clean: ## Cleanup runtime files
	rm -rf triebwerk *.out

clean-all: clean ## Cleanup ALL runtime files
	rm -rf triebwerk

.PHONY: help all run tools deps fmt-check fmt lint vet test test-unit test-short wasm integration-test cover cover-html build build-static loadtest loadtest-check clean clean-all
