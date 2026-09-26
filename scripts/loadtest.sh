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

# the server needs Google credentials to start, it never uses them with the fake master
openssl genrsa 2048 2>/dev/null > "$work/key.pem"
python3 - "$work" <<'PY'
import json, sys
work = sys.argv[1]
json.dump({
    "type": "service_account", "project_id": "triebwerk-local", "private_key_id": "local",
    "private_key": open(work + "/key.pem").read(),
    "client_email": "local@triebwerk-local.iam.gserviceaccount.com", "client_id": "1",
    "token_uri": "https://oauth2.googleapis.com/token",
}, open(work + "/credentials.json", "w"))
PY

"$work/fakemaster" -addr :8081 > "$work/fakemaster.log" 2>&1 &
GOOGLE_APPLICATION_CREDENTIALS="$work/credentials.json" FIRESTORE_EMULATOR_HOST=localhost:8999 \
  PUBLIC_IP=localhost PORT=9090 REGION=EU MASTERSERVER_GRPC=localhost:8081 \
  "$work/triebwerk" > "$work/server.log" 2>&1 &
server=$!
sleep 2

echo "server log: $work/server.log"
"$work/loadtest" -url ws://localhost:9090/echo -view :8090 "$@" &
wait -n
kill -0 $server 2>/dev/null || { echo "server stopped:"; tail -40 "$work/server.log"; exit 1; }
