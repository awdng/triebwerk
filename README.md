# triebwerk
triebwerk is a simple multiplayer game server written in Golang.

Warning: Very experimental!

Build and run triebwerk with docker:
docker-compose up

Run triebwerk standalone:
cp .env.dist .env (and change env values)
make run
The server waits until it reaches the master server at MASTER_URL (http://localhost:8081),
locally start the fake one first: go run ./cmd/fakemaster -addr :8081

The master server (its own repository, ../master):
The master lists the game servers for the clients, has the accounts of the
players and keeps the scores of every round. A game server talks to it over
HTTP (infra/master_server.go): it registers under PUBLIC_IP:PORT with its
name, map, player limit and protocol version, reports every five seconds who
is online and the scores of its round (and once more when a round ends),
asks who the token of a new player belongs to, registers again when the
master has forgotten it, and says goodbye when it stops.
MASTER_URL    address of the master, default http://localhost:8081
SERVER_KEY    what the master asks game servers for, if it does
SERVER_NAME   name in the list of servers, default "<REGION> <PUBLIC_IP>:<PORT>"
A client says hello with a token. A ticket of the master (pt_...) makes the
player an account: the client got it from the master for this one join, and
the game server never sees the session behind it. Any other token is the
name of a guest. An account plays one match at a time:
the master turns it away while it is in a match on another server, and on
one server a second connection of an account takes the place of the first
(a second window, a reload). The server tells the master at once when an
account leaves, and drops the accounts a heartbeat's answer names. A player the master turns away is told
why (message Denied) before the connection is closed.
cmd/fakemaster speaks the same API, accepts every server, takes every player
for a guest by the name given and keeps nothing: the tests and scripts of this repository need no master.
`make stack` in ../master runs the real thing: master, several game servers
and bots. Bots play as accounts when they are given tickets (cmd/loadtest
-tokens), which is what the master's end to end test does.

Watching a server:
Every game server shows its match from above at http://<address>/spectate/
(the live view of the load test, fed from the server's own state, package
spectate): tanks and shots of every tick, who plays with which keys and
score. With a SERVER_KEY it needs ?key=, which the admin pages of the master
server hand out. Eight watchers at most; nobody watching costs nothing.

Build triebwerk:
make build-static

GAME_LENGTH (seconds) sets the round length, 600 (ten minutes) by default;
short rounds are useful for tests.

How the server keeps time and talks to clients:
The game moves in ticks of 33 ms. Every tick the server applies one input per
player, moves every tank, shell and timer by one step and sends every player a
snapshot. Inputs are numbered and wait in a queue for their tick; when a
player's input is late its tick is bridged with the keys of the last one (for
at most ten ticks), so a tank moves the same for everybody whatever its
client's connection does. Late inputs that would only wait behind the ticks
that were bridged for them are dropped (the burst after a frame that took too
long); one is kept in reserve. A snapshot tells its receiver which of its
inputs it contains, bridged ticks included, and how many wait, which is what
clients predict and pace themselves with.
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
By default it serves http by IP address and the game connects over the
WebSocket. QA_TLS=on make qa asks Tailscale for a certificate (HTTPS has to be
enabled for the tailnet): everything is then served over https under the
machine's tailnet name and the game connects over WebTransport, falling back
to the WebSocket when UDP does not get through. The first certificate makes
the machine's tailnet name public for good (certificates are listed in public
logs), which is why it has to be asked for.
Ports and options are listed at the top of scripts/qa.sh.

Clients get the names, kills and deaths of all players in a scoreboard message
(once a second and after every change), followed by a message that lists who
of them plays with an account (everybody else is a guest) and one that says
what everybody drives, and a kill message when a tank is destroyed.

Kinds of tanks:
There are seven (model/tanks.go, `Tanks`): how fast each drives and turns,
how fast its turret turns, how long it reloads, what a hit of its gun takes
and how much of a hit its armour lets through, and how big it is for
collisions and hits. Health is 0 to 100 for all of them. The first kind is
the tank the game always had; it is what everybody drives who says nothing.
A client says what it wants with a Loadout message (kind and skin): the
first one of a connection counts at once if the tank still stands where it
was put (clients say it right behind their hello), every other one when the
tank is next put on the map (respawn, start of a round). The skin is a number the server
only passes on. The table is also what the browser predicts its own tank
with: cmd/wasm hands it out (`tankSpecs`), so change the numbers in one
place and rebuild the wasm (`make wasm`).
The bots of the load test take the kinds in turn (`-one-kind` for the
first kind only). The load test bots read both and the checks fail when a player
never receives a scoreboard, when a message does not have the agreed layout
or when the server never applies a player's inputs.

The browser client in `../client` connects to the same server, see its README.
The load test only needs Go, the fake master server accepts every player so
the nickname entered in the client is used as its name.
