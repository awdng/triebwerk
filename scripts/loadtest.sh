#!/usr/bin/env bash
# Runs a local load test: fake master server, game server and simulated
# players. Open http://localhost:8090 to watch the match.
# Extra arguments are passed to the load test, e.g. -n 12 -d 10m -reconnect 3
set -euo pipefail

cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$work"' EXIT

go build -o "$work/triebwerk" ./cmd/server
go build -o "$work/fakemaster" ./cmd/fakemaster
go build -o "$work/loadtest" ./cmd/loadtest

"$work/fakemaster" -addr :8081 > "$work/fakemaster.log" 2>&1 &
PUBLIC_IP=localhost PORT=9090 REGION=EU MASTERSERVER_GRPC=localhost:8081 GAME_LENGTH="${GAME_LENGTH:-300}" \
  "$work/triebwerk" > "$work/server.log" 2>&1 &
server=$!
sleep 2

echo "server log: $work/server.log"
"$work/loadtest" -url ws://localhost:9090/echo -view :8090 "$@" &
wait -n
kill -0 $server 2>/dev/null || { echo "server stopped:"; tail -40 "$work/server.log"; exit 1; }
