# triebwerk
triebwerk is a simple multiplayer game server written in Golang.

Warning: Very experimental!

Build and run triebwerk with docker:
docker-compose up

Run triebwerk standalone:
cp .env.dist .env (and change env values)
make run
The server waits until it reaches the master server at MASTERSERVER_GRPC,
locally start the fake one first: go run ./cmd/fakemaster -addr :8081

Build triebwerk:
make build-static

GAME_LENGTH (seconds) sets the round length, useful for tests.

How the server keeps time and talks to clients:
The game moves in ticks of 33 ms. Every tick the server applies one input per
player, moves every tank, shell and timer by one step and sends every player a
snapshot. Inputs are numbered and wait in a queue for their tick; when a
player's input is late the last one is applied again (for at most ten ticks),
so a tank moves the same for everybody whatever its client's connection does.
A snapshot tells its receiver which of its inputs it contains and how many
wait, which is what clients predict and pace themselves with.
The wire format is described at the top of protocol/binary.go (version 2: a
client says hello with its version and is welcomed or rejected). It has two
lanes: a reliable one for what must arrive (hello, rounds, scoreboard, kills)
and an unreliable one for what the next message replaces anyway (inputs,
snapshots, pings).

Connections (package transport):
Clients connect over a WebSocket at /echo, where both lanes share the socket.
With a certificate the server also offers WebTransport at /wt on the same
port number over UDP: the reliable lane is a stream, the unreliable lane are
datagrams, and a lost packet no longer holds up the snapshots behind it.
GET /transport tells clients what a server offers.
TLS_CERT and TLS_KEY (files of a certificate for PUBLIC_IP, which is a host
name then) turn on TLS for the WebSocket (wss) and WebTransport.
WEBTRANSPORT_SELF_SIGNED=true offers WebTransport without a certificate
file: the server makes one itself and tells clients its hash, by which
Chromium accepts it. The WebSocket stays without TLS. For development and
tests; the load test script and the client's e2e suite use it.

Run tests:
make test          (formatting, vet, race tests and a 12 player load test in process, about a minute)
make test-short    (without the load test)
make loadtest-check (real binaries, 12 simulated players through a round, fails on drops, stalls
                    or inputs that arrive too late)
make loadtest-check-webtransport (the same with the players connected over WebTransport)

The race detector needs cgo and therefore a C compiler (gcc, e.g.
apt install gcc). goimports is pinned in go.mod as a tool and runs through
`go tool goimports`, there is nothing to install.

The load test players live in the loadtest package, TestLoad12Players in
game/loadtest_test.go and cmd/loadtest -check use the same checks
(loadtest.Report.Check). CI runs all of it on every push, see
.github/workflows/ci.yml.
Run a local load test with a live view of the match:
make loadtest (then open http://localhost:8090)

This starts a fake master server (cmd/fakemaster), the game server and 12 simulated
players (cmd/loadtest) that drive around and shoot with random inputs. Options can be
passed to the script directly, e.g. ./scripts/loadtest.sh -n 12 -d 10m -reconnect 3
It uses the ports 8081 (master), 9090 (game) and 8090 (live view), MASTER_PORT,
GAME_PORT and VIEW_PORT change them. loadtest-check runs on 28081 and 29090 so
it does not collide with a running local game.
TRANSPORT=webtransport connects the players over WebTransport instead of the
WebSocket. cmd/loadtest takes either address: -url ws://host:port/echo or
-url https://host:port/wt (it does not check certificates).

QA environment for other machines, e.g. a desktop in the same Tailscale network:
make qa
It builds this server and the client in ../client, starts the fake master
server, the game server, 2 simulated players and the production client build,
and prints the URLs. Ctrl-C stops everything.
If Tailscale gives this machine a certificate (HTTPS enabled for the tailnet),
everything is served over https under the machine's tailnet name and the game
connects over WebTransport, falling back to the WebSocket when UDP does not
get through. The first certificate makes the machine's tailnet name public
(certificates are listed in public logs); QA_TLS=off does without and serves
http by IP address with the WebSocket only, as does a tailnet without HTTPS.
Ports and options are listed at the top of scripts/qa.sh.

Clients get the names, kills and deaths of all players in a scoreboard message
(once a second and after every change) and a kill message when a tank is
destroyed. The load test bots read both and the checks fail when a player
never receives a scoreboard, when a message does not have the agreed layout
or when the server never applies a player's inputs.

The browser client in `../client` connects to the same server, see its README.
The load test only needs Go, the fake master server accepts every player so
the nickname entered in the client is used as its name.
