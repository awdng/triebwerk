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

Run tests:
make test          (formatting, vet, race tests and a 12 player load test in process, about a minute)
make test-short    (without the load test)
make loadtest-check (real binaries, 12 simulated players through a round, fails on drops or stalls)

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

QA environment for other machines, e.g. a desktop in the same Tailscale network:
make qa
It builds this server and the client in ../client, starts the fake master
server, the game server, 2 simulated players and the production client build,
and prints the URLs for this machine's Tailscale IP. Ctrl-C stops everything.
Ports and options are listed at the top of scripts/qa.sh.

Clients get the names, kills and deaths of all players in a scoreboard message
(once a second and after every change) and a kill message when a tank is
destroyed; the byte layout is documented in protocol/binary.go. The load test
bots read both and the checks fail when a player never receives a scoreboard.

The browser client in `../client` connects to the same server, see its README.
The load test only needs Go, the fake master server accepts every player so
the nickname entered in the client is used as its name.
