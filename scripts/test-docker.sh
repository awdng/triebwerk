#!/usr/bin/env bash
# Runs the tests with the race detector in a Go container, for machines
# without a C compiler (the race detector needs cgo). Modules and the Go
# toolchain that go.mod pins come from this machine's module cache, mounted
# into the container; the source is mounted read only.
# Extra arguments go to `go test`, e.g. ./scripts/test-docker.sh -short
set -euo pipefail

cd "$(dirname "$0")/.."
image=${GO_IMAGE:-golang:1.24}
module_cache=$(go env GOMODCACHE)
packages='$(go list -e ./... | grep -v cmd/wasm)'

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOFLAGS=-buildvcs=false -e GOTOOLCHAIN=auto \
  -v "$(dirname "$module_cache")":/go/pkg \
  -v "$PWD":/src:ro -w /src \
  "$image" sh -c "go test -race $* $packages"
