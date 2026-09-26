# triebwerk
triebwerk is a simple multiplayer game server written in Golang.

Warning: Very experimental!

Build and run triebwerk with docker:
docker-compose up

Run triebwerk standalone:
cp .env.dist .env (and change env values)
make run

Build triebwerk:
make build-static

GAME_LENGTH (seconds) sets the round length, useful for tests.

Run tests:
make test
Run a local load test with a live view of the match:
make loadtest (then open http://localhost:8090)

This starts a fake master server (cmd/fakemaster), the game server and 12 simulated
players (cmd/loadtest) that drive around and shoot with random inputs. Options can be
passed to the script directly, e.g. ./scripts/loadtest.sh -n 12 -d 10m -reconnect 3

The browser client in `../client` connects to the same server, see its README.
The load test only needs Go, the fake master server accepts every player so
the nickname entered in the client is used as its name.
