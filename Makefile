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
	GO111MODULE=on $(GO) get golang.org/x/tools/cmd/goimports
	GO111MODULE=on $(GO) get -u golang.org/x/lint/golint

fmt-check: ## Check formatting (goimports)
	@ goimports -e -d -l $(GOFILES)

fmt: ## Fix formatting (goimports)
	@ goimports -w $(GOFILES)

lint: ## Perform lint checks
ifndef HAS_GOLINT
	$(GO) get github.com/golang/lint/golint
endif
	golint -set_exit_status $(GOPACKAGES)

vet: ## Perform vet checks
	go vet $(GOPACKAGES)

test: fmt-check vet test-unit ## Execute all checks and tests

test-unit: ## Execute unit tests
	$(GO) test -race -v $(GOPACKAGES)

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

clean: ## Cleanup runtime files
	rm -rf triebwerk *.out

clean-all: clean ## Cleanup ALL runtime files
	rm -rf triebwerk

.PHONY: help all run tools deps fmt-check fmt lint vet test test-unit integration-test cover cover-html build build-static clean clean-all
