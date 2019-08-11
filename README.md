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

Run tests:
make test