package game

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/awdng/triebwerk/model"
)

// time between the end of a game and the start of the next one
const intermission = 10 * time.Second

// A connection that has not said hello after this many ticks (30 s) is
// closed. Generous on purpose: a browser says hello from its main thread,
// which can be busy for seconds while a page starts. A socket that is dead
// is found much earlier by the pings of the network manager.
const helloTimeoutTicks = 30000 / model.TickMillis

// events that are created outside of the game loop and processed inside of it
type joinEvent struct {
	conn model.Connection
}

type leaveEvent struct {
	conn model.Connection
}

// accounts the master server wants dropped: they play on another server
type kickEvent struct {
	accounts []string
}

type authEvent struct {
	player   *model.Player
	globalID string
	nickname string
	err      error
}

// Controller ...
// The game state is owned by the game loop goroutine, other goroutines
// communicate with it through events.
type Controller struct {
	tickStart      time.Time
	networkManager *NetworkManager
	state          *model.GameState
	masterServer   MasterServerClient
	// who watches the server live, nil if nobody can
	spectators Spectators
	maxPlayers int
	// connections that have not said hello yet: they hold a place on the
	// server but are no tanks and are not sent anything
	waiting   []*waitingClient
	nextStart time.Time
	// the scoreboard is sent when it changed and once a second anyway
	scoreboardDirty bool
	scoreboardSent  time.Time

	eventMutex sync.Mutex
	events     []interface{}

	snapshotMutex sync.Mutex
	snapshot      model.StateSnapshot

	// An account left during this tick. The master is told at once, so the
	// player can join another server without waiting for the next heartbeat.
	accountLeft bool
	reportNow   chan struct{}

	// how long ticks take: their number and the sum in nanoseconds, kept by
	// the game loop
	ticksMeasured int64
	tickTimeTotal int64
}

type waitingClient struct {
	client *model.Client
	// tick the connection arrived at
	since uint32
}

// Spectators are shown the state of the game after every tick. Publish is
// called by the game loop and must not hold it up.
type Spectators interface {
	Publish(state *model.GameState)
}

// MasterServerClient ...
type MasterServerClient interface {
	Init(address string)
	// SendHeartbeat returns the accounts that are to be dropped because
	// they are in a match on another server already
	SendHeartbeat(model.StateSnapshot) []string
	EndGame(model.StateSnapshot)
	AuthorizePlayer(token string) (globalID string, nickname string, err error)
}

// NewController creates a game instance
func NewController(region string, maxPlayers int, gameLength time.Duration, networkManager *NetworkManager, masterServer MasterServerClient) *Controller {
	state := model.NewGameState(region, gameLength)
	return &Controller{
		networkManager: networkManager,
		state:          state,
		masterServer:   masterServer,
		maxPlayers:     maxPlayers,
		snapshot:       state.Snapshot(),
		reportNow:      make(chan struct{}, 1),
	}
}

// Watch lets spectators see the game, to be called before Init
func (g *Controller) Watch(spectators Spectators) {
	g.spectators = spectators
}

// RegisterPlayer registers a networked Player
func (g *Controller) RegisterPlayer(conn model.Connection) {
	g.pushEvent(joinEvent{conn: conn})
}

// UnregisterPlayer of a networked game
func (g *Controller) UnregisterPlayer(conn model.Connection) {
	g.pushEvent(leaveEvent{conn: conn})
}

// pushEvent never blocks, so it is safe to call from the network goroutines
func (g *Controller) pushEvent(event interface{}) {
	g.eventMutex.Lock()
	defer g.eventMutex.Unlock()
	g.events = append(g.events, event)
}

func (g *Controller) popEvents() []interface{} {
	g.eventMutex.Lock()
	defer g.eventMutex.Unlock()
	events := g.events
	g.events = nil
	return events
}

// Init the gameserver
func (g *Controller) Init() error {
	// init HeartBeat
	go g.HeartBeat()

	go g.gameLoop()

	// Start networking
	return g.networkManager.Start()
}

// heartbeatInterval is how often the master server hears from this one.
// An account that leaves is reported at once, but not more often than
// every reportGap.
const (
	heartbeatInterval = 5 * time.Second
	reportGap         = time.Second
)

// HeartBeat registers the server with the master and reports to it for as
// long as the server runs
func (g *Controller) HeartBeat() {
	// Wait for Network to become ready
	time.Sleep(time.Second)

	ticker := time.NewTicker(heartbeatInterval)
	g.masterServer.Init(g.networkManager.GetAddress())
	var last time.Time
	for {
		select {
		case <-ticker.C:
		case <-g.reportNow:
			if wait := reportGap - time.Since(last); wait > 0 {
				time.Sleep(wait)
			}
		}
		last = time.Now()
		g.dropAccounts(g.masterServer.SendHeartbeat(g.latestSnapshot()))
	}
}

// dropAccounts turns away the players of accounts that the master found
// on another server as well. Safe to call from any goroutine.
func (g *Controller) dropAccounts(accounts []string) {
	if len(accounts) > 0 {
		g.pushEvent(kickEvent{accounts: accounts})
	}
}

// reportWanted says, once, that the master should hear from the server now
func (g *Controller) reportWanted() bool {
	select {
	case <-g.reportNow:
		return true
	default:
		return false
	}
}

func (g *Controller) latestSnapshot() model.StateSnapshot {
	g.snapshotMutex.Lock()
	defer g.snapshotMutex.Unlock()
	return g.snapshot
}

func (g *Controller) handleEvents() {
	for _, event := range g.popEvents() {
		switch e := event.(type) {
		case joinEvent:
			g.addConnection(e.conn)
		case leaveEvent:
			g.removePlayer(e.conn)
		case authEvent:
			g.finishAuthorization(e)
		case kickEvent:
			g.kick(e.accounts)
		}
	}
}

// addConnection takes a new connection in. It becomes a player when it
// says hello, see greet.
func (g *Controller) addConnection(conn model.Connection) {
	if g.state.GetPlayerCount()+len(g.waiting) >= g.maxPlayers {
		log.Printf("GameManager: Rejecting connection %s, server is full (%d Players)", conn.Identifier(), g.maxPlayers)
		conn.Close(writeWait, true)
		conn.Close(writeWait, false)
		return
	}
	client := model.NewClient(conn)
	g.networkManager.Register(client)
	g.waiting = append(g.waiting, &waitingClient{client: client, since: g.state.TickNumber()})
}

// greet reads what the connections that are not players yet have sent. The
// first hello decides: a client that speaks this server's version becomes a
// player, any other is turned away. Who says nothing for too long is closed.
func (g *Controller) greet() {
	waiting := g.waiting[:0]
	for _, w := range g.waiting {
		hello, said := firstHello(w.client)
		switch {
		case said && hello.Version == g.networkManager.Version():
			g.addPlayer(w.client, hello.Token)
		case said:
			log.Printf("GameManager: Rejecting connection %s, it speaks version %d of the protocol", w.client.Connection.Identifier(), hello.Version)
			g.networkManager.Reject(w.client)
		case g.state.TickNumber()-w.since > helloTimeoutTicks:
			log.Printf("GameManager: Closing connection %s, it never said hello", w.client.Connection.Identifier())
			g.networkManager.Disconnect(w.client)
		default:
			waiting = append(waiting, w)
		}
	}
	for i := len(waiting); i < len(g.waiting); i++ {
		g.waiting[i] = nil
	}
	g.waiting = waiting
}

// firstHello takes messages of a client until it finds a hello. Whatever
// was sent before it is dropped, what comes after it is left for the player.
func firstHello(client *model.Client) (model.Hello, bool) {
	for len(client.NetworkIn) != 0 {
		message := <-client.NetworkIn
		if hello, ok := message.Body.(model.Hello); ok {
			return hello, true
		}
	}
	return model.Hello{}, false
}

func (g *Controller) addPlayer(client *model.Client, token string) {
	players := g.state.GetPlayers()
	pID := g.state.GetNewPlayerID()
	spawn := g.state.Map.GetRandomSpawn(players)
	player := model.NewPlayer(pID, spawn.X, spawn.Y, client.Connection)
	player.Client = client
	// the welcome is the first thing a player hears
	g.networkManager.Welcome(player, g.state)
	g.state.AddPlayer(player)
	if g.state.InProgress() { // game already started, new Player has to know about it
		g.networkManager.SendRoundStart(player.Client, g.state)
	}
	g.authorize(player, token)
	g.scoreboardDirty = true
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.GetPlayerCount())
}

func (g *Controller) removePlayer(conn model.Connection) {
	for i, w := range g.waiting {
		if w.client.Connection == conn {
			g.waiting = append(g.waiting[:i], g.waiting[i+1:]...)
			return
		}
	}
	for _, p := range g.state.GetPlayers() {
		if p.Client.Connection == conn {
			g.state.RemovePlayer(p)
			g.scoreboardDirty = true
			g.accountLeft = g.accountLeft || p.GlobalID != ""
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.GetPlayerCount())
			return
		}
	}
}

// kick turns away the players of accounts that play on another server
func (g *Controller) kick(accounts []string) {
	for _, p := range g.state.GetPlayers() {
		for _, account := range accounts {
			if p.GlobalID != "" && p.GlobalID == account {
				log.Printf("GameManager: Player %d (%s) plays on another server as well, dropping it here", p.ID, p.GlobalID)
				g.networkManager.Deny(p.Client, model.DeniedElsewhere)
			}
		}
	}
}

// authorize asks the master server in the background so the game loop does not wait for it
func (g *Controller) authorize(p *model.Player, token string) {
	go func() {
		globalID, nickname, err := g.masterServer.AuthorizePlayer(token)
		g.pushEvent(authEvent{player: p, globalID: globalID, nickname: nickname, err: err})
	}()
}

func (g *Controller) finishAuthorization(e authEvent) {
	p := e.player
	if !g.isConnected(p) {
		return
	}
	if e.err != nil {
		log.Printf("GameManager: Player %d could not be authorized, forcing disconnect: %s", p.ID, e.err)
		reason := model.DeniedNoMaster
		switch {
		case errors.Is(e.err, model.ErrNotAuthorized):
			reason = model.DeniedNotAuthorized
		case errors.Is(e.err, model.ErrPlayingElsewhere):
			reason = model.DeniedElsewhere
		}
		g.networkManager.Deny(p.Client, reason)
		return
	}
	p.GlobalID = e.globalID
	p.Nickname = e.nickname
	g.scoreboardDirty = true
	// An account has one tank. A second window, or the page after a reload
	// while the old connection still hangs on, takes the place of the first.
	if p.GlobalID != "" {
		for _, other := range g.state.GetPlayers() {
			if other != p && other.GlobalID == p.GlobalID {
				log.Printf("GameManager: Player %d joined again as Player %d (%s), dropping the older connection", other.ID, p.ID, p.GlobalID)
				g.networkManager.Deny(other.Client, model.DeniedReplaced)
			}
		}
	}
	log.Printf("GameManager: Player %d authorized successfully as GlobalID %s %s", p.ID, p.GlobalID, p.Nickname)
}

func (g *Controller) isConnected(player *model.Player) bool {
	for _, p := range g.state.GetPlayers() {
		if p == player {
			return true
		}
	}
	return false
}

// readMessages takes what arrived from a player since the last tick. Inputs
// are queued for their ticks, they do not move anything here. A loadout is
// what the player wants to drive.
func (g *Controller) readMessages(p *model.Player) {
	for len(p.Client.NetworkIn) != 0 {
		message := <-p.Client.NetworkIn
		switch body := message.Body.(type) {
		case []model.Controls:
			for _, input := range body {
				p.Inputs.Push(input)
			}
		case model.Loadout:
			// the tank it joins with at once, another one at the next spawn
			p.Choose(body)
		}
	}
}

// noteLoadouts sees to it that the clients hear of a tank that changed:
// the one a player joined with, or the one it came back as
func (g *Controller) noteLoadouts(players []*model.Player) {
	for _, p := range players {
		if p.LoadoutChanged {
			p.LoadoutChanged = false
			g.scoreboardDirty = true
		}
	}
}

// reportKills tells the clients who was destroyed in this tick
func (g *Controller) reportKills(players []*model.Player) {
	for _, p := range players {
		for _, victim := range p.TakeKills() {
			g.networkManager.BroadcastKill(g.state, p.ID, victim)
			g.scoreboardDirty = true
		}
	}
}

// sendScoreboard sends names and scores when they changed, and once a
// second so that clients that just joined or missed one catch up
func (g *Controller) sendScoreboard() {
	if !g.scoreboardDirty && g.tickStart.Sub(g.scoreboardSent) < time.Second {
		return
	}
	g.networkManager.BroadcastScoreboard(g.state)
	g.scoreboardDirty = false
	g.scoreboardSent = g.tickStart
}

func (g *Controller) startGame() {
	g.state.Start()
	g.scoreboardDirty = true
	g.networkManager.BroadcastRoundStart(g.state)
	log.Printf("GameManager: Game has started")
}

func (g *Controller) endGame() {
	g.state.End()
	log.Printf("GameManager: Game has ended")
	g.networkManager.BroadcastRoundEnd(g.state)
	go g.masterServer.EndGame(g.state.Snapshot())
	g.nextStart = g.tickStart.Add(intermission)
}

// gameLoop is the only goroutine that modifies the game state
func (g *Controller) gameLoop() {
	ticker := time.NewTicker(model.TickMillis * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		start := time.Now()
		g.tick(start)

		g.ticksMeasured++
		g.tickTimeTotal += int64(time.Since(start))
	}
}

// tick is one step of the server: take in who came, went and what was sent,
// move the world by one step and tell every player what it looks like now
func (g *Controller) tick(now time.Time) {
	g.tickStart = now
	g.handleEvents()
	g.greet()

	if !g.state.InProgress() && g.state.ReadyToStart() && !g.tickStart.Before(g.nextStart) {
		g.startGame()
	}

	players := g.state.GetPlayers()
	for _, p := range players {
		g.readMessages(p)
	}
	// one step for everybody, with the input each client sent for it
	g.state.ApplyInputs()
	g.state.Tick()
	g.noteLoadouts(players)

	if g.state.InProgress() {
		g.networkManager.SendSnapshots(g.state)
		g.reportKills(players)
		g.sendScoreboard()

		if g.state.HasEnded() {
			g.endGame()
		}
	}

	g.snapshotMutex.Lock()
	g.snapshot = g.state.Snapshot()
	g.snapshotMutex.Unlock()
	// only now: the report is made of the snapshot that was just taken
	if g.accountLeft {
		g.accountLeft = false
		select {
		case g.reportNow <- struct{}{}:
		default: // a report is asked for already
		}
	}

	if g.spectators != nil {
		g.spectators.Publish(g.state)
	}
}
