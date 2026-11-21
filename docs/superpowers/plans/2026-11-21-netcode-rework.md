# Netcode Rework Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan phase by phase. Each phase gets its task-level breakdown (files, interfaces, test code) when it starts; the phases below fix what is built, in which order, and how it is proven.

**Goal:** Remote tanks move smoothly and the own tank feels exact on any connection, with a server that alone decides how fast things move.

**Architecture:** The server simulates every tank and shell once per fixed tick and consumes exactly one input per player per tick from a small queue. Clients run the same step function on the same ticks, so prediction is exact except on contact with other tanks. Snapshots are stamped with tick numbers and remote tanks are drawn a little in the past, interpolated between them. Messages travel on two lanes, unreliable (inputs, snapshots) and reliable (events); WebSocket carries both today, WebTransport datagrams plus a stream carry them at the end.

**Tech Stack:** Go server (`server/`), the model compiled to WebAssembly for the client (`server/cmd/wasm` → `client/public/game/tanks.wasm`), JavaScript client (`client/src/game/net`, `sim`), Vitest, Go tests, Playwright. New at the end: `quic-go/webtransport-go`, HTTPS in the QA setup.

**Spec:** the Audit and Design sections below. Brief: the user's request of 2026-11-21 — two computers, tanks close to each other, "the network player skips around slightly" under the best latency; rework client side prediction and server authoritative movement; evaluate QUIC based transport instead of WebSocket.

## Audit: what the netcode does today

Measured on 2026-11-21 with two browsers and the real server (script: turn a tank on the spot, record what the server broadcasts for it and what the other client draws).

| # | Finding | Evidence |
|---|---|---|
| 1 | **The server moves a tank once per input message, not once per tick.** The tick only drains the inbox (`game/controller.go` `processInputs` calls `p.Update` per message). How far a tank moves in a tick is how many of its messages happened to arrive in it: 0, 1 or 2. | With the driver's input timer at 34 ms against the server's 33 ms tick, 17 of 605 broadcasts (2.8 %) showed no movement at all; the watching client drew the tank standing still in 1.7 % of frames and more than 50 % off its speed in 3.1 %. With both at exactly 33 ms on one machine: 0 %. Two computers never share a clock, and a busy browser delivers timers late. |
| 2 | **Speed and fire rate follow the message rate.** | Server test: 30 input messages move a tank 14.8 units, 60 messages 29.7. A client that sends faster drives and reloads faster; a slow one (low frame rate, throttled tab) is slow. |
| 3 | **A destroyed browser player does not come back until the round restarts.** The respawn countdown only advances inside `p.Update`, and the browser stops sending inputs while its tank is destroyed (`Game.networkTick`). The load test bots never stop sending, so no test saw it. | Server test: dead for 20 s of ticks without input messages, countdown still 0. Client side read in the code; I could not produce a kill in a headless match to watch it end to end. |
| 4 | **Shells only fly while their owner sends inputs** (`HandleWeapons` is inside `p.Update`). | Server test: a shell stays where it was fired until more inputs arrive. |
| 5 | **The client predicts on its own clock.** `Prediction.update` applies the latest input once per 33 ms of *render* time; inputs are recorded and sent by a separate `setInterval`. Steps simulated and inputs sent are different counts, so prediction and server disagree by construction and the difference is hidden by blending 10 % towards the server per update. | Code. On one machine the two clocks agree and corrections measured 0; the design only works while they do. |
| 6 | **Snapshots are stamped with the wall clock at encoding time**, each player's entry separately, not with the tick they describe. | `GameState.GameTime()` in `BroadcastGameState`. |
| 7 | **Remote tanks interpolate linearly 100 ms in the past**, angles without wrap, delay fixed whatever the jitter. Shots of remote tanks are spawned when the message arrives, 100 ms ahead of the tank they come out of. | `sim/Interpolation.js`, `Game.applyEntry`. With clean server data the drawn speed had a standard deviation of 5 %: the interpolation itself is not the problem. |
| 8 | **One TCP stream for everything.** A lost packet holds back every later one until it is retransmitted. Irrelevant on a wired LAN or Tailscale without loss (the skipping reproduces without any), real on WiFi and mobile. | Not measured yet, Phase 0 adds loss to the test conditions. |
| 9 | No protocol version, client and server use different byte orders, a shot is one flag in one snapshot (lost if that snapshot is). | `protocol/binary.go`, `net/protocol.js`. |

Finding 1 is the skipping. Findings 2 to 4 are the same design decision seen from other sides. None of them is the transport.

## Design

**Server, fixed tick.** Every tick, for every player: take the next input from that player's queue; if none has arrived, repeat the last one and count the tick as starved. Simulate one step. Then move every shell and run every respawn timer once. Never more than one step per player per tick; a queue longer than 4 drops its oldest entries. Game time is the tick number times the step.

**Inputs.** Numbered per client tick. Every packet carries the last three inputs, so a lost packet loses nothing. The server reports in each snapshot, per client, the last input it applied and how many were waiting in the queue.

**Client, same ticks.** One fixed step loop, driven by an accumulator on frame time (several ticks in one slow frame, so a low frame rate no longer means fewer inputs) and by a timer while the tab is hidden. Per tick: read the keys, number the input, send it, simulate one step, remember the predicted state under that number. The loop runs up to 2 % faster or slower to keep about 1.5 inputs waiting on the server.

**Reconciliation.** Compare the server's state for input N with the remembered prediction for N. Equal: nothing happens. Different: rewind to the server's state, replay the inputs after N, and let the *drawn* position slide to the new one over about 100 ms. The simulation is never blended.

**Remote tanks.** Snapshots are buffered by tick. Drawn at server time minus a delay of two ticks plus the measured jitter, adapted slowly; angles the short way around; at most two ticks of extrapolation. Shots are spawned when the drawn time reaches their tick, from a per-player shot counter instead of a one-tick flag.

**Two lanes.** Unreliable and newest-wins: inputs up, snapshots down. Reliable and ordered: hello with protocol version and tick rate, round start and end, scoreboard, kills. One interface on each side with two implementations: WebSocket (both lanes on the one stream) and WebTransport (datagrams and one stream).

**Transport.** WebTransport when the browser has it, the page is served over HTTPS and the connection comes up within two seconds; otherwise WebSocket. Both stay supported.

### QUIC in 2026: what is true

- WebTransport (HTTP/3 over QUIC, unreliable datagrams plus streams) is in every current browser since Safari 26.4 shipped it in March 2026; Chrome has had it since 97, Firefox since 114. It is "newly available", not yet everywhere: older Safari and networks that block UDP need the WebSocket fallback.
- It needs HTTPS: a page on `http://100.x.y.z` cannot open one. On the tailnet, `tailscale cert` issues a real certificate for the machine's `ts.net` name; `make qa` has to serve the client and the game over it. Pinning a self-signed certificate (`serverCertificateHashes`) is reliable in Chromium; Firefox has open bugs on it and Safari is unconfirmed, so it is a test tool, not a setup for playing.
- In Go, `quic-go/webtransport-go` implements it, datagrams included; use v0.10.0 or later (two denial of service fixes in 2026). Its README has said the project is unfunded and loosely maintained: check the state before Phase 4, the alternative is WebRTC data channels with `pion`, which works but needs signalling.
- The underlying spec is still an IETF draft in last call.
- What it buys: no head of line blocking under packet loss, and a connection that survives a network change. What it does not buy: anything on a loss free link. It is the last phase for that reason.

## Global Constraints

- The server alone decides movement: one simulated step per player per tick, whatever a client sends.
- Client prediction runs the server's Go model as WebAssembly; a change to `server/model` or `server/cmd/wasm` means `make wasm` and committing `client/public/game/tanks.wasm`.
- Byte layouts live in `server/protocol/binary.go`, `client/src/game/net/protocol.js` and the load test bot (`server/loadtest/bot.go`); all three change together, with their tests.
- Every phase ends green: server `make test` (or the no-race fallback), `make loadtest-check`, client `yarn lint && yarn test`, `E2E_SERVER_SOURCE=local npm test`.
- The screenshot tests stay reproducible: nothing visual reads the wall clock.
- 60 FPS: `PERF_MIN_FPS=58 npm run perf` keeps passing; the client tick loop must not cost frames.

## Acceptance: the numbers that have to move

| Measure | Today | Target | Phase |
|---|---|---|---|
| Remote tank turning, driver's inputs every 34 ms: frames drawn standing still | 1.7 % | 0 | 1 |
| Same: frames more than 50 % off the right speed | 3.1 % | under 0.3 % | 2 |
| Destroyed player whose client sends nothing is back after 3 s | never | yes | 1 |
| Distance covered in 1 s at 60 input messages per second | 2.0x | 1.0x | 1 |
| Shell keeps flying when its owner stops sending | no | yes | 1 |
| Own tank, 100 ms latency with 40 ms jitter: corrections that are visible (over 0.5 units) per minute while driving free | measured in Phase 0 | 0 | 2 |
| 5 % packet loss, 60 ms latency: longest time a remote tank is drawn without new data | measured in Phase 0 | 100 ms | 4 |

## Review Focus

- **A client that sends nothing, garbage, or 1000 inputs per second:** the server keeps ticking at its rate, nobody else stutters, the sender gains nothing. Tests in Phase 1.
- **A client far ahead or behind** (laptop woke up, tab was hidden for minutes): it resynchronises within a second instead of replaying or fast forwarding. Phase 2.
- **Input numbers wrapping or restarting** (reconnect, round restart): no stale acknowledgement is applied. Phase 2.
- **Reordered and duplicated datagrams:** an older snapshot never overwrites a newer one, a repeated input is applied once. Phase 3.
- **WebTransport that connects and then loses its datagram path** (UDP blocked mid-session): the game falls back instead of freezing. Phase 4.

---

## Phase 0: Make it measurable

Nothing changes for players. Everything below is a test or a tool.

- [ ] `server`: a per-connection network condition simulator behind the `Transport` (delay, jitter, loss, duplication, reordering; set by environment variables, off by default). The client's `LatencySimulator` gets loss and reordering for the unreliable lane.
- [ ] `client/e2e/netcode.js` (`npm run netcode`): the measurement used for the audit as a tool. Two browsers; one turns and drives at constant input, the other records what the server reports and what it draws; the driver's own corrections. Options for input cadence, latency, jitter, loss. Prints the acceptance table.
- [ ] `client`: a net overlay with `?net`: round trip, jitter, loss, snapshot rate, inputs waiting on the server, corrections per second and their size, interpolation delay.
- [ ] Failing tests that pin the audit, each named for the behaviour wanted: server `TestDestroyedPlayerReturnsWithoutInputs`, `TestShellsFlyWithoutInputs`, `TestSpeedDoesNotDependOnMessageRate`; e2e `remote tank moves evenly when the driver's inputs come late`. They stay red until Phase 1 and are skipped in CI until then, with the finding number in the skip reason.
- [ ] Fill the two "measured in Phase 0" cells of the acceptance table.

## Phase 1: The server ticks

The wire format does not change, so the current client keeps working and this ships on its own. This phase removes the skipping.

- [ ] `model`: `Player.Step(input, world, dt)` simulates one tick of one player; `GameState.Tick()` steps every player, every shell and every respawn timer once. Shells move in `Tick`, not in a player's update.
- [ ] `game`: a per-player input queue (capacity 4, newest wins on overflow, duplicates and older numbers dropped). The loop takes one input per player per tick and repeats the last one when the queue is empty; starved ticks are counted per player and logged once a second when above zero.
- [ ] Game time is `tick * step`; all entries of one broadcast carry the same time.
- [ ] Tests: the three server tests from Phase 0 go green; one input per tick whatever arrives (0, 1, 5, 1000 messages in a tick give the same movement); an input message that is malformed or far in the future is ignored; `TestLoad12Players` and `make loadtest-check` pass with the bots unchanged.
- [ ] `wasm`: the client's prediction calls the same `Step`; rebuild and commit `tanks.wasm`.
- [ ] Acceptance rows marked Phase 1.

Known cost, fixed in Phase 2: when the server repeats an input the driver's own prediction is one step short and gets corrected. The watcher's view is already smooth.

## Phase 2: The client ticks

- [ ] `sim/TickLoop.js`: the fixed step loop (accumulator, at most 5 ticks per frame, then drop; timer while hidden) with a rate that can be nudged by ±2 %. Tested without a browser: frames of 5, 16.7, 40 and 300 ms give the same number of ticks per second.
- [ ] `sim/Prediction.js` rewritten: predicted state remembered per input number; `reconcile` compares before it rewinds; a drawn offset that decays instead of blending the simulation. Tested: identical server state means no change at all; a different one means the simulation equals the replayed truth immediately and the drawn pose arrives within 150 ms.
- [ ] Clock: the server adds to every snapshot, per client, the number of inputs waiting when it took one. The client steers its tick rate to hold 1.5. This is the first wire change: one byte in the state message, old clients ignore it.
- [ ] `sim/Interpolation.js`: buffered by tick, delay of two ticks plus measured jitter (p95 over 2 s, never less than two ticks, changes by at most 2 % of playback speed), angles the short way, extrapolation of at most two ticks. Tested with recorded arrival patterns: steady, jittered, a burst after a gap.
- [ ] Remote shots are spawned when the drawn time reaches the snapshot they belong to.
- [ ] The client sends inputs while its tank is destroyed and while the tab is hidden.
- [ ] Acceptance rows marked Phase 2, under clean conditions and under 100 ms ± 40 ms.

## Phase 3: Protocol version 2, two lanes

Still over WebSocket. Breaking: client, server and bots change together.

- [ ] Hello on connect: protocol version, tick rate, round length, own id. A version the server does not speak gets a clear "update the game" message instead of garbage.
- [ ] Little endian in both directions.
- [ ] Input packet: newest input number plus three inputs as bit fields (about 8 bytes).
- [ ] Snapshot: tick, per receiver the acknowledged input and inputs waiting, then per tank id, position, two angles, health and a shot counter (about 14 bytes each, 12 tanks fit in a fifth of one datagram).
- [ ] Reliable lane: hello, round start and end, scoreboard, kills.
- [ ] `Lane` interface on both sides with the WebSocket implementation; snapshots older than the newest seen are dropped, repeated inputs applied once.
- [ ] Bots speak version 2; the load test fails on starved ticks on localhost.
- [ ] Tests for every layout on all three implementations, including truncated and oversized messages.

## Phase 4: WebTransport

- [ ] Check the state of `webtransport-go` first; if it is abandoned, stop and decide between a fork, WebRTC data channels, or staying on WebSocket.
- [ ] Server: an HTTP/3 listener next to the WebSocket one, datagrams for the unreliable lane, one bidirectional stream for the reliable lane; certificate and key from the environment.
- [ ] Client: `net/WebTransportLane.js`; `Connection` tries it when `window.WebTransport` exists and the page is a secure context, and falls back to WebSocket on failure, on a two second timeout, or when datagrams stop arriving for a second while the stream is alive.
- [ ] `make qa`: serves the client over HTTPS with the certificate from `tailscale cert` and passes it to the game server; prints the `https://<machine>.<tailnet>.ts.net` link. Without a certificate it says so and runs as today.
- [ ] e2e: the gameplay suite runs once per transport (Chromium, self-signed certificate pinned by hash).
- [ ] Acceptance row marked Phase 4, WebSocket and WebTransport side by side at 5 % loss.

## Phase 5: Decide after playing

Not planned in detail, each needs a playtest on the result of Phases 1 to 4 first.

- Lag compensation for hits (the server rewinds targets to what the shooter saw). Shells are slow, so leading the target is part of the game; this may not be wanted.
- 60 ticks per second instead of 30.
- Delta compressed snapshots, only needed far beyond 12 players.

## Results (2026-11-21)

Phases 0 to 4 are implemented on the `netcode-rework` branches of both
repositories. Measured with `npm run netcode` in `client/e2e` (two browsers on
the Intel UHD 630 of the development machine, 8 s per scenario) and by the
tests named.

| Measure | Before | Target | Now |
|---|---|---|---|
| Remote tank turning, driver's clock 3 % slow: frames drawn standing still | 1.7 % | 0 | 0.0 % |
| Same: frames more than 50 % off the right speed | 3.1 % | under 0.3 % | 0.0 % (0.2 % in one of four runs) |
| Destroyed player whose client sends nothing is back after 3 s | never | yes | yes (`TestDestroyedPlayerReturnsWithoutInputs`) |
| Distance covered in 1 s at 60 input messages per second | 2.0x | 1.0x | 1.0x (`TestSpeedDoesNotDependOnMessageRate`, e2e "twice as many inputs") |
| Shell keeps flying when its owner stops sending | no | yes | yes (`TestShellsFlyWithoutInputs`) |
| Own tank, 100 ms latency with 40 ms jitter: corrections | not comparable: the old prediction tolerated 0.1 units of error and blended the rest | 0 visible | 0 of any size |
| 5 % packet loss, 60 ms latency: longest time a remote tank stands | not measured before | 100 ms | 0 ms, drawn 80 to 110 ms in the past |
| How far in the past other tanks are drawn on a quiet line | 100 ms, fixed | as little as the line allows | 41 ms |

The numbers are the same over WebSocket and WebTransport. The loss in these
measurements is simulated in the client (messages are dropped before they are
sent or after they arrived), which does not reproduce what a lost packet does
to a TCP connection: there, everything behind it waits. What WebTransport gains
on a line that really loses packets is therefore not measured here; that needs
`tc netem` on the server (root) or a bad line.

Not verified: WebTransport from a browser to a server with a certificate signed
by an authority (the QA environment over `tailscale cert`). Verified instead:
the same from a browser with a self signed certificate accepted by its hash
(e2e), from the Go bots with a certificate from files, and a browser that falls
back to the encrypted WebSocket when it does not trust the certificate.

## Decisions taken during implementation

The plan was implemented without further questions, as asked. Where it was
silent or turned out wrong, this is what was decided, and what it costs if
the decision was wrong.

- **Order of the phases: 0, 1, 3, 2, 4.** The client's tick loop needed the acknowledged snapshots of protocol version 2. No cost.
- **Snapshots keep float32 positions and angles.** Reconciliation compares predicted and authoritative state; quantised values would have made every comparison approximate. Cost: 19 bytes per tank instead of about 10.
- **A connection is a player only after its hello** (was: on connect), has 30 seconds to say it and holds a place meanwhile. Cost: twelve silent sockets can hold a full server for 30 seconds; before, any twelve sockets held it for good.
- **The hello carries the auth token to the end of the message**, without a length byte: tokens of the master server can be longer than 255 bytes.
- **Pings are answered by the connection's reader**, not by the game loop: the round trip a client shows no longer contains up to 33 ms of waiting for a tick.
- **Input pacing looks at the fewest inputs that waited in the last half second** (the reserve), not at the average: a client sends the ticks of a frame in one go. Cost: on a jittery line more input delay than an average would give.
- **Late inputs that would only wait are dropped** (added after the final review): the snapshot's acknowledgement counts a bridged tick as the input that was late for it, and when more late inputs arrive than one for the coming tick and one in reserve, the older ones are dropped. Before, a burst after a long frame waited in the queue for seconds. A single late input is still applied and becomes the reserve; dropping those too was tried and made a client with a slow clock starve five times as often. Cost: after a long frame the keys of the dropped inputs are never applied.
- **The server bridges at most ten ticks**, then the keys count as released. Cost: a line that stalls for more than a third of a second stops its tank for the others.
- **The input queue holds eight inputs** (plan: four): at ten frames per second three arrive together and one should be in reserve.
- **Other tanks are drawn at least 1.25 ticks in the past** (plan: never under two), one tick more when snapshots are lost. Measured: no frame without data at 41 ms on a quiet line.
- **Everything in a frame uses the timestamp the browser gives the frame**, not the time the code runs at. Found as the cause of uneven drawing in the measurements under load.
- **The collision boxes of other tanks are turned in the browser's copy of the simulation** (`RectCollider.SetPose`). They never were; not in the plan, found while reading the wasm bridge.
- **The turret keys are no longer crossed** between client and server (a known server quirk), fixed in the model instead of on the wire.
- **No network simulator on the server.** The client's simulator drives the tests. Cost: the loss it simulates does not reproduce what a lost packet does to TCP, so what WebTransport gains on a line that loses packets is not measured (see Results).
- **Snapshots are still only sent while a round runs.** The client forgets inputs and interpolation at the end of a round.
- **The netcode tests in the browser do not draw** (`?render=off`): two software rendering browsers next to each other reach a few frames per second, and what is measured is where tanks are, not what they look like. Gameplay and screenshot tests still draw.
- **WebTransport with a certificate the server makes itself** (`WEBTRANSPORT_SELF_SIGNED`) keeps the WebSocket without TLS: browsers accept a certificate by its hash only for WebTransport. For development and tests.
- **When WebTransport stops working in the middle of a match the client joins again over the WebSocket as a new player**: after two seconds without a snapshot, or when the session ends by itself. Cost: the score of the round is lost.
- **`make qa` asks Tailscale for a certificate only with `QA_TLS=on`.** The first certificate makes the machine's tailnet name public for good; the question that was answered with yes did not say so. No certificate was requested during this work.
- **After a pause of 0.9 seconds or more the client's tick loop runs one tick** and starts over, instead of catching up.
- **A page that is left closes its connection** (`pagehide`), and the menu's form connects on its own only once per page load. Found when a killed browser kept its place on a full server for six seconds over WebTransport.
- **Not built from the plan:** a log line per second for bridged ticks (the load test and the `?net` overlay report them instead); the gameplay e2e suite as a separate Playwright project per transport (it runs over WebTransport in the main run and over the WebSocket in the latency run).

## Decisions for the user

1. **Order.** Fix the simulation first, transport last, as above. The measured skipping does not involve the transport; WebTransport helps on lossy links only.
2. **HTTPS in QA.** WebTransport needs it. The tailnet already offers this server a certificate domain (checked 2026-11-21, no certificate was requested). Is opening the game through the `ts.net` name instead of the IP acceptable?
3. **Compatibility window.** Phase 3 breaks old clients. Simplest: merge server and client together and pin the client's CI to the matching server branch for that one change. Alternative: the server speaks both versions for a while, at the cost of carrying the old code.
4. **Starting point.** The `visual-overhaul` branches are not merged. This work should start from them merged, or branch off them.
