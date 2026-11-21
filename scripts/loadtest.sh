#!/usr/bin/env bash
# Runs a local load test: fake master server, game server and simulated
# players. Open http://localhost:8090 to watch the match.
# Extra arguments are passed to the load test, e.g. -n 12 -d 10m -reconnect 3
# MASTER_PORT, GAME_PORT and VIEW_PORT change the ports (8081, 9090, 8090).
# TRANSPORT=webtransport lets the players connect over WebTransport (UDP on
# the game port) instead of the websocket; the server then runs with a
# certificate of its own.
set -uo pipefail

cd "$(dirname "$0")/.."
master_port=${MASTER_PORT:-8081}
game_port=${GAME_PORT:-9090}
view_port=${VIEW_PORT:-8090}
transport=${TRANSPORT:-websocket}
case "$transport" in
  websocket) url="ws://localhost:$game_port/echo" ;;
  webtransport) url="https://localhost:$game_port/wt" ;;
  *) echo "TRANSPORT is websocket or webtransport, not $transport" >&2; exit 1 ;;
esac

port_open() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

# a server that cannot listen exits at once, say why instead
for port in "$master_port" "$game_port"; do
  if port_open "$port"; then
    echo "port $port is already in use; stop that process or set MASTER_PORT / GAME_PORT" >&2
    exit 1
  fi
done

work=$(mktemp -d)
trap 'kill $(jobs -p) 2>/dev/null; rm -rf "$work"' EXIT

# the logs are deleted on exit, show them when something went wrong
fail() {
  echo "$1" >&2
  tail -n 40 "$work/fakemaster.log" "$work/server.log" >&2
  exit 1
}

go build -o "$work/triebwerk" ./cmd/server || exit 1
go build -o "$work/fakemaster" ./cmd/fakemaster || exit 1
go build -o "$work/loadtest" ./cmd/loadtest || exit 1

"$work/fakemaster" -addr ":$master_port" > "$work/fakemaster.log" 2>&1 &
master=$!
PUBLIC_IP=localhost PORT="$game_port" REGION=EU MASTER_URL="http://localhost:$master_port" GAME_LENGTH="${GAME_LENGTH:-600}" \
  WEBTRANSPORT_SELF_SIGNED=true \
  "$work/triebwerk" > "$work/server.log" 2>&1 &
server=$!

# the game server opens its port once it reached the master server
for _ in $(seq 100); do
  port_open "$game_port" && break
  kill -0 "$server" 2>/dev/null || fail "the game server stopped during startup:"
  sleep 0.1
done
port_open "$game_port" || fail "the game server did not open port $game_port within 10s:"

echo "server log: $work/server.log"
"$work/loadtest" -url "$url" -view ":$view_port" "$@" &
wait -n
status=$?
if ! kill -0 "$server" 2>/dev/null || ! kill -0 "$master" 2>/dev/null; then
  fail "a server stopped during the load test:"
fi
exit "$status"
