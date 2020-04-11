package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go"
	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/infra"
	"github.com/awdng/triebwerk/protocol"
	websocket "github.com/awdng/triebwerk/transport"
	"github.com/kelseyhightower/envconfig"

	pb "github.com/awdng/triebwerk-proto/gameserver"
	"google.golang.org/grpc"
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

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithBlock())
	opts = append(opts, grpc.WithInsecure())
	opts = append(opts, grpc.WithTimeout(5*time.Second))

	var conn *grpc.ClientConn
	for conn == nil {
		conn, err = grpc.Dial(config.MasterServerGRPC, opts...)
		if err != nil {
			log.Printf("failed to connect to GRPC backend: %v", err)
			log.Println("Retrying...")
			time.Sleep(2 * time.Second)
		}
	}

	defer conn.Close()
	pbclient := pb.NewGameServerMasterClient(conn)
	masterServer := infra.NewMasterServerClient(pbclient)

	log.Printf("Loading Triebwerk ...")

	playerManager := game.NewPlayerManager(firebase)
	transport := websocket.NewTransport(config.PublicIP, config.Port)
	networkManager := game.NewNetworkManager(transport, protocol.NewBinaryProtocol())
	controller := game.NewController(config.Region, networkManager, playerManager, firebase, masterServer)
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
