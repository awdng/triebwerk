#!/usr/bin/env bash
# QA environment for other machines, e.g. a desktop in the same Tailscale
# network: fake master server, game server, simulated players and the
# production build of ../client, all from the working copies on disk.
# Ctrl-C stops everything.
#
# By default the game is served over http by IP address and connects over
# the WebSocket. With a certificate for this machine's name in the tailnet
# everything is served over https and the game server offers WebTransport
# (UDP on the game port) as well. Tailscale hands out such certificates
# (`tailscale cert`) when HTTPS is enabled for the tailnet. They come from
# Let's Encrypt, which lists every certificate in public logs: the machine's
# tailnet name becomes public with the first one, for good. So that is asked
# for, not assumed: QA_TLS=on.
#
# Settings (environment):
#   QA_TLS       on: ask Tailscale for a certificate and serve https
#                (see above); default: http
#   QA_CERT, QA_KEY  certificate files to use instead of asking Tailscale;
#                QA_HOST has to be the name they are for
#   QA_HOST      address in the printed URLs   default: the tailnet name with
#                a certificate, else this machine's Tailscale IPv4
#   CLIENT_PORT  browser client                38080
#   GAME_PORT    game server (WebSocket, and WebTransport over UDP)  39090
#   VIEW_PORT    live view of the bots         38090
#   MASTER_PORT  fake master server, local     38081
#   BOTS         simulated players, 0 for none 2
#   GAME_LENGTH  round length in seconds       300
set -uo pipefail

server_dir=$(cd "$(dirname "$0")/.." && pwd)
client_dir=$(cd "$server_dir/../client" 2>/dev/null && pwd) || {
  echo "expected the client checkout next to the server: $server_dir/../client" >&2
  exit 1
}

client_port=${CLIENT_PORT:-38080}
game_port=${GAME_PORT:-39090}
view_port=${VIEW_PORT:-38090}
master_port=${MASTER_PORT:-38081}
bots=${BOTS:-2}
tls=${QA_TLS:-off}
cert=${QA_CERT:-}
key=${QA_KEY:-}
host=${QA_HOST:-}

# logs, binaries and the client build; kept after exit for a look at the logs
work=${TMPDIR:-/tmp}/panzr-qa
mkdir -p "$work/bin"

# the name Tailscale can get a certificate for, empty if there is none
tailnet_name() {
  tailscale status --json 2>/dev/null \
    | sed -n '/"CertDomains"/,/\]/{/CertDomains/d;s/^ *"\([^"]*\)".*/\1/p;}' | head -n 1
}

if [ "$tls" = on ] && [ -z "$cert" ]; then
  name=$(tailnet_name)
  if [ -z "$name" ]; then
    echo "QA_TLS=on, but Tailscale has no certificate name for this machine (is HTTPS enabled for the tailnet?)" >&2
    exit 1
  fi
  if [ -n "$host" ] && [ "$host" != "$name" ]; then
    echo "QA_TLS=on serves under $name; QA_HOST=$host does not fit" >&2
    exit 1
  fi
  mkdir -p "$work/tls"
  chmod 700 "$work/tls"
  echo "asking Tailscale for the certificate of $name"
  if ! tailscale cert --cert-file "$work/tls/$name.crt" --key-file "$work/tls/$name.key" "$name" > "$work/tailscale-cert.log" 2>&1; then
    echo "no certificate from Tailscale:" >&2
    sed 's/^/  /' "$work/tailscale-cert.log" >&2
    exit 1
  fi
  cert="$work/tls/$name.crt"
  key="$work/tls/$name.key"
  host=$name
elif [ "$tls" != on ] && [ "$tls" != off ]; then
  echo "QA_TLS is on or off, not $tls" >&2
  exit 1
fi
if [ -n "$cert" ] && { [ ! -r "$cert" ] || [ ! -r "$key" ]; }; then
  echo "can not read the certificate files QA_CERT=$cert QA_KEY=$key" >&2
  exit 1
fi
if [ -n "$cert" ] && [ -z "$host" ]; then
  echo "QA_CERT needs QA_HOST: the name the certificate is for" >&2
  exit 1
fi

host=${host:-$(tailscale ip -4 2>/dev/null | head -n 1)}
if [ -z "$host" ]; then
  echo "no Tailscale IPv4 address found (tailscale ip -4), set QA_HOST to the address to use" >&2
  exit 1
fi
if [ -n "$cert" ]; then scheme=https; socket=wss; else scheme=http; socket=ws; fi

port_open() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

wait_for_port() { # port, process id, name
  for _ in $(seq 150); do
    port_open "$1" && return 0
    kill -0 "$2" 2>/dev/null || fail "$3 stopped during startup:" "$3"
    sleep 0.1
  done
  fail "$3 did not open port $1 within 15s:" "$3"
}

fail() { # message, log names
  echo "$1" >&2
  shift
  for name in "$@"; do tail -n 30 "$work/$name.log" >&2; done
  exit 1
}

ports=("$client_port" "$game_port" "$master_port")
[ "$bots" -gt 0 ] && ports+=("$view_port")
for port in "${ports[@]}"; do
  if port_open "$port"; then
    echo "port $port is already in use; stop that process or choose another port (see $0)" >&2
    exit 1
  fi
done

echo "building the server from $server_dir"
(cd "$server_dir" && go build -o "$work/bin/" ./cmd/server ./cmd/fakemaster ./cmd/loadtest) || exit 1

echo "building the client from $client_dir"
if [ ! -d "$client_dir/node_modules" ]; then
  if command -v yarn >/dev/null; then yarn=(yarn); else yarn=(corepack yarn); fi
  (cd "$client_dir" && COREPACK_ENABLE_DOWNLOAD_PROMPT=0 "${yarn[@]}" install --frozen-lockfile) || exit 1
fi
vite="$client_dir/node_modules/.bin/vite"
# its own output directory, client/dist belongs to the e2e suite and yarn build
(cd "$client_dir" && "$vite" build --logLevel warn --outDir "$work/dist" --emptyOutDir) || exit 1

trap 'kill $(jobs -p) 2>/dev/null' EXIT
# Ctrl-C reaches every process, stop quietly instead of reporting them
trap 'exit 0' INT TERM

declare -A pids
start() { # name, command...
  local name=$1
  shift
  "$@" > "$work/$name.log" 2>&1 &
  pids[$name]=$!
}

start fakemaster "$work/bin/fakemaster" -addr "127.0.0.1:$master_port"
wait_for_port "$master_port" "${pids[fakemaster]}" fakemaster

# with a certificate the server speaks TLS and offers WebTransport
start server env PUBLIC_IP="$host" PORT="$game_port" REGION=EU MASTERSERVER_GRPC="127.0.0.1:$master_port" \
  GAME_LENGTH="${GAME_LENGTH:-300}" MAX_PLAYERS=12 TLS_CERT="$cert" TLS_KEY="$key" "$work/bin/server"
wait_for_port "$game_port" "${pids[server]}" server

if [ "$bots" -gt 0 ]; then
  start bots "$work/bin/loadtest" -n "$bots" -url "$socket://127.0.0.1:$game_port/echo" -view ":$view_port"
  wait_for_port "$view_port" "${pids[bots]}" bots
fi

start client env PREVIEW_TLS_CERT="$cert" PREVIEW_TLS_KEY="$key" PREVIEW_HOST="$host" \
  "$vite" preview "$client_dir" --outDir "$work/dist" --host 0.0.0.0 --port "$client_port" --strictPort
wait_for_port "$client_port" "${pids[client]}" client

cat <<EOF

QA environment is running, open on your desktop:

  game        $scheme://$host:$client_port/?server=$host:$game_port&name=desktop
              (or $scheme://$host:$client_port and connect to $host:$game_port)
EOF
[ "$bots" -gt 0 ] && echo "  bots view   http://$host:$view_port   ($bots simulated players)"
cat <<EOF

  Every browser window is one player. Add &latency=100&jitter=40&loss=5 to
  the game URL to simulate a worse network, &net to see what the netcode
  measures. Rounds last ${GAME_LENGTH:-300}s.
EOF
if [ -n "$cert" ]; then
  cat <<EOF
  The game connects over WebTransport (UDP port $game_port) and falls back to
  the WebSocket (TCP) when that does not get through, e.g. a firewall that
  lets only TCP pass; &net shows which one it is.
EOF
else
  hint=$(tailnet_name)
  cat <<EOF
  Without https the game connects over the WebSocket only. WebTransport
  needs a certificate${hint:+: QA_TLS=on make qa asks Tailscale for one and
  serves under $hint. That name is public from then on}, see the top of
  $0.
EOF
fi
cat <<EOF
  Logs: $work/*.log    Ctrl-C stops everything.
EOF

wait -n
for name in "${!pids[@]}"; do
  kill -0 "${pids[$name]}" 2>/dev/null || fail "the $name process stopped:" "$name"
done
