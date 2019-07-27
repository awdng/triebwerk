package game

import (
	"context"
	"log"
	"time"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/model"
)

const tickrate = 30

var numMeasurements int64
var totalMeasurement int64
var avgTickTime float64

type serverState struct {
	Connect   string         `firestore:"connect"`
	Scores    map[string]int `firestore:"scores"`
	GameTime  int            `firestore:"gametime"`
	UpdatedAt int64          `firestore:"updated_at"`
}

// Controller ...
type Controller struct {
	tickStart      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          *model.GameState
	firebase       *triebwerk.Firebase
}

// NewController creates a game instance
func NewController(networkManager *NetworkManager, playerManager *PlayerManager, firebase *triebwerk.Firebase) *Controller {
	return &Controller{
		networkManager: networkManager,
		playerManager:  playerManager,
		state:          model.NewGameState(),
		firebase:       firebase,
	}
}

// RegisterPlayer registers a networked Player
func (g *Controller) RegisterPlayer(conn model.Connection) {
	players := g.state.GetPlayers()
	pID := g.state.GetNewPlayerID()
	spawn := g.state.Map.GetRandomSpawn(players)
	player := model.NewPlayer(pID, spawn.X, spawn.Y, conn)
	g.networkManager.Register(player, g.state)
	g.state.AddPlayer(player)
	g.Start()
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.GetPlayerCount())
}

// UnregisterPlayer of a networked game
func (g *Controller) UnregisterPlayer(conn model.Connection) {
	players := g.state.GetPlayers()
	for _, p := range players {
		if p.Client.Connection == conn {
			g.state.RemovePlayer(p)
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.GetPlayerCount())
			break
		}
	}
}

// Init the gameserver
func (g *Controller) Init() error {
	// goroutine constantly reads player input
	go g.processInputs()

	// init HeartBeat
	go g.HeartBeat()

	// Start networking
	return g.networkManager.Start()
}

// HeartBeat ...
func (g *Controller) HeartBeat() {
	// Wait for Network to become ready
	time.Sleep(time.Second)

	ctx := context.Background()
	server, _, err := g.firebase.Store.Collection("Server").Add(ctx, g.buildServerState())
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(time.Second * 5)
	for range ticker.C {
		_, err := server.Set(ctx, g.buildServerState())
		if err != nil {
			log.Printf(err.Error())
		}
	}
}

func (g *Controller) buildServerState() serverState {
	players := g.state.GetPlayers()
	scores := map[string]int{}
	for _, p := range players {
		scores[p.GlobalID] = p.Score
	}
	serverState := serverState{
		Connect:   g.networkManager.GetAddress(),
		UpdatedAt: time.Now().UTC().Unix(),
		GameTime:  int(g.state.GameTime()),
		Scores:    scores,
	}
	return serverState
}

// Start the gameserver loop
func (g *Controller) Start() {
	if g.state.ReadyToStart() {
		// ctx := context.Background()
		// _, _, err := g.database.Collection("Match").Add(ctx, g.state.GetAsMap())
		// if err != nil {
		// 	log.Fatal(err)
		// }

		g.state.Start()
		// Execute game loop
		go g.gameLoop()
	}
}

func (g *Controller) processInputs() {
	// continously read all player inputs at 1000Hz
	ticker := time.NewTicker(1 * time.Millisecond)
	for range ticker.C {
		players := g.state.GetPlayers()
		for _, p := range players {
			select {
			case message := <-p.Client.NetworkIn:
				switch messageType := message.MessageType; messageType {
				case 0:
					token := message.Body.(string)
					err := g.playerManager.Authorize(p, token)
					if err != nil {
						log.Printf("GameManager: Player %d could not be verified, forcing disconnect: %s", p.ID, err)
						g.networkManager.ForceDisconnect(p)
						continue
					}
					log.Printf("GameManager: Player %d authorized successfully as GlobalID %s", p.ID, p.GlobalID)
				case 1:
					p.Control = message.Body.(model.Controls)
				case 5:
					g.networkManager.SendTime(p, g.state, &message)
				default:
				}
			default:
			}
		}
	}
}

func (g *Controller) gameLoop() {
	gameEnded := make(chan bool)
	interval := time.Duration(int(1000/tickrate)) * time.Millisecond
	timestep := float32(interval/time.Millisecond) / 1000

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("GameManager: Game has started")
	for {
		select {
		case <-ticker.C:
			g.tickStart = time.Now()
			players := g.state.GetPlayers()

			// apply latest client inputs
			for _, p := range players {
				p.Update(players, g.state, timestep)
				p.HandleRespawn(g.state)
			}

			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)

			if g.state.HasEnded() {
				close(gameEnded)
			}

			// measure average tick time
			numMeasurements++
			totalMeasurement += time.Now().UTC().UnixNano() - g.tickStart.UTC().UnixNano()
			avgTickTime = float64(totalMeasurement/numMeasurements) / 1000 / 1000
		case _, ok := <-gameEnded:
			if !ok {
				g.state.End()
				log.Printf("GameManager: Game has ended")
				time.Sleep(10 * time.Second)
				g.Start()
				return
			}
		}
	}
}
