package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	firebase "firebase.google.com/go"
	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	f, _ := os.Create("../goroutine_profile")
	defer f.Close()

	// pprof.StartCPUProfile(f)
	// defer func() {
	// 	log.Println("stop cpu profile")
	// 	pprof.StopCPUProfile()
	// }()
	//runtime.SetMutexProfileFraction(1000)
	profile := pprof.Lookup("goroutine")
	defer profile.WriteTo(f, 1)
	//runtime.SetBlockProfileRate(1000)

	var config triebwerk.Config
	// load env vars into config struct
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
