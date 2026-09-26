package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/infra"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"

	pb "github.com/awdng/triebwerk-proto/gameserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// waitForConnection blocks until the master server is reachable
func waitForConnection(conn *grpc.ClientConn) {
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return
		}
		if state == connectivity.TransientFailure {
			log.Printf("failed to connect to GRPC backend %s, retrying...", conn.Target())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn.WaitForStateChange(ctx, state)
		cancel()
	}
}

func main() {
	// load env vars into config struct
	var config triebwerk.Config
	if err := envconfig.Process("", &config); err != nil {
		log.Fatal(err)
	}

	conn, err := grpc.NewClient(config.MasterServerGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	waitForConnection(conn)

	defer conn.Close()
	pbclient := pb.NewGameServerMasterClient(conn)
	masterServer := infra.NewMasterServerClient(pbclient)

	log.Printf("Loading Triebwerk ...")

	transport := websocket.NewTransport(config.PublicIP, config.Port)
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := game.NewController(config.Region, config.MaxPlayers, time.Duration(config.GameLength)*time.Second, networkManager, masterServer)
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
