package infra

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/awdng/panzr-api/gameserver"
	"github.com/awdng/triebwerk/model"
)

// MasterServerClient ...
type MasterServerClient struct {
	grpcClient pb.GameServerMasterClient
	address    string
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
}

// GetServerState ...
func (m *MasterServerClient) GetServerState() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	state, err := m.grpcClient.GetServerState(ctx, &pb.ServerStateRequest{
		State: &pb.ServerState{
			Address: m.address,
		},
	})
	if err != nil {
		log.Println("Error Receiving ServerState - %v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	fmt.Println("We got this server state on init", state)
}

// SendHeartbeat ...
func (m *MasterServerClient) SendHeartbeat(gameState *model.GameState) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	state, err := m.grpcClient.SendHeartbeat(ctx, &pb.ServerStateRequest{
		State: buildServerState(gameState, m.address),
	})
	if err != nil {
		log.Println("Error Sending ServerState - %v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	fmt.Println(state)
}

func buildServerState(gameState *model.GameState, address string) *pb.ServerState {
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
		Address:     address,
		UpdatedAt:   int32(time.Now().UTC().Unix()),
		ElapsedTime: int32(gameState.GameTime()),
		Players:     players,
	}
}
