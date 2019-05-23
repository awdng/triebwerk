package main

import (
	"log"

	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
)

func main() {
	log.Printf("Loading Triebwerk ...")
	networkManager := game.NewNetworkManager(websocket.NewTransport(), protocol.NewBinaryProtocol())
	log.Fatal(networkManager.Start())
}
