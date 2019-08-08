package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	firebase "firebase.google.com/go"
	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	// load env vars into config struct
	var config triebwerk.Config
	if err := envconfig.Process("", &config); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	firebaseConfig := &firebase.Config{}

	app, err := firebase.NewApp(ctx, firebaseConfig)
	if err != nil {
		log.Fatal(err)
	}

	client, err := app.Firestore(ctx)
	if err != nil {
		log.Fatal(err)
	}

	firebase := &triebwerk.Firebase{
		App:   app,
		Store: client,
	}

	log.Printf("Loading Triebwerk ...")

	playerManager := game.NewPlayerManager(firebase)
	transport := websocket.NewTransport(config.PublicIP)
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := game.NewController(networkManager, playerManager, firebase)
	transport.RegisterNewConnHandler(controller.RegisterPlayer)
	transport.UnregisterConnHandler(controller.UnregisterPlayer)

	go func() {
		// start game server
		log.Fatal(controller.Init())
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	s := <-sigs
	log.Printf("shutdown with signal %s", s)
}
