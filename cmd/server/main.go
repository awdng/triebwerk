package main

import (
	"log"

	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
)

func main() {
	log.Printf("Loading Triebwerk ...")

	playerManager := game.NewPlayerManager()
	transport := websocket.NewTransport()
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	gameManager := game.NewGame(networkManager, playerManager)
	transport.RegisterNewConnHandler(gameManager.RegisterPlayer)
	transport.UnregisterConnHandler(gameManager.UnregisterPlayer)

	// start game server
	log.Fatal(gameManager.Start())
}
