package infra

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/awdng/triebwerk-proto/gameserver"
	"github.com/awdng/triebwerk/model"
)

// MasterServerClient ...
type MasterServerClient struct {
	grpcClient pb.GameServerMasterClient
	address    string
	id         string
}

// NewMasterServerClient ...
func NewMasterServerClient(grpc pb.GameServerMasterClient) *MasterServerClient {
	return &MasterServerClient{
		grpcClient: grpc,
	}
}

// Init ...
func (m *MasterServerClient) Init(address string) {
	m.address = address
	m.registerServer()
}

// GetServerState ...
func (m *MasterServerClient) GetServerState() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	state, err := m.grpcClient.GetServerState(ctx, &pb.GetServerRequest{
		Id: m.id,
	})
	if err != nil {
		log.Println("Error Receiving ServerState - %v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	fmt.Println(state)
}

// RegisterServer ...
func (m *MasterServerClient) registerServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	server, err := m.grpcClient.RegisterServer(ctx, &pb.ServerRegisterRequest{
		Address: m.address,
	})
	if err != nil {
		log.Println("Error Registering Server - %v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	m.id = server.Id
}

// SendHeartbeat ...
func (m *MasterServerClient) SendHeartbeat(gameState *model.GameState) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	state, err := m.grpcClient.SendHeartbeat(ctx, &pb.ServerStateRequest{
		State: m.buildServerState(gameState),
	})
	if err != nil {
		log.Println("Error Sending ServerState - %v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	fmt.Println(state)
}

func (m *MasterServerClient) buildServerState(gameState *model.GameState) *pb.ServerState {
	statePlayers := gameState.GetPlayers()

	players := []*pb.Player{}
	for _, pd := range statePlayers {
		p := &pb.Player{
			Name:  pd.Nickname,
			Score: int32(pd.Score),
			Team:  0,
		}
		players = append(players, p)
	}

	return &pb.ServerState{
		Id:          m.id,
		Address:     m.address,
		UpdatedAt:   int32(time.Now().UTC().Unix()),
		ElapsedTime: int32(gameState.GameTime()),
		Players:     players,
	}
}
