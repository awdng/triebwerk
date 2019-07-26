package main

import (
	"log"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	var config triebwerk.Config
	// load env vars into config struct
	if err := envconfig.Process("", &config); err != nil {
		log.Fatal(err)
	}

	log.Printf("Loading Triebwerk ...")

	playerManager := game.NewPlayerManager()
	transport := websocket.NewTransport(config.PublicIP)
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := game.NewController(networkManager, playerManager)
	transport.RegisterNewConnHandler(controller.RegisterPlayer)
	transport.UnregisterConnHandler(controller.UnregisterPlayer)

	// start game server
	log.Fatal(controller.Init())
}
