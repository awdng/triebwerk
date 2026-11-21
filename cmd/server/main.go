package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/infra"
	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
	"github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"
)

// set at build time (-ldflags), see the Makefile
var (
	CommitHash = "dev"
	Tag        = ""
)

func main() {
	// load env vars into config struct
	var config triebwerk.Config
	if err := envconfig.Process("", &config); err != nil {
		log.Fatal(err)
	}

	masterServer := infra.NewMasterServerClient(infra.MasterConfig{
		URL:        config.MasterURL,
		Key:        config.ServerKey,
		Name:       config.ServerName,
		Region:     config.Region,
		Map:        model.MapName,
		MaxPlayers: config.MaxPlayers,
		Secure:     config.TLSCert != "",
		Protocol:   protocol.Version,
		Version:    CommitHash,
	})
	// a server that can not ask the master who a player is can let nobody in
	masterServer.WaitUntilReachable()

	log.Printf("Loading Triebwerk ...")

	clients := transport.New(transport.Config{
		Address:    config.PublicIP,
		Port:       config.Port,
		CertFile:   config.TLSCert,
		KeyFile:    config.TLSKey,
		SelfSigned: config.WebTransportSelfSigned,
	})
	networkManager := game.NewNetworkManager(clients, protocol.NewBinaryProtocol())
	controller := game.NewController(config.Region, config.MaxPlayers, time.Duration(config.GameLength)*time.Second, networkManager, masterServer)
	clients.RegisterNewConnHandler(controller.RegisterPlayer)
	clients.UnregisterConnHandler(controller.UnregisterPlayer)

	go func() {
		// start game server
		log.Fatal(controller.Init())
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	s := <-sigs
	log.Printf("shutdown with signal %s", s)
	// leave the list of servers now, not after the heartbeats it will miss
	masterServer.Close()
}
