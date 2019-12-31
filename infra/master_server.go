package infra

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/awdng/panzr-api/gameserver"
)

// MasterServer ...
type MasterServer struct {
	grpcClient pb.GameServerMasterClient
}

// NewMasterServer ...
func NewMasterServer(grpc pb.GameServerMasterClient) *MasterServer {
	return &MasterServer{
		grpcClient: grpc,
	}
}

// GetHeartBeat ...
func (m *MasterServer) GetHeartBeat() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	state, err := m.grpcClient.GetServerState(ctx, &pb.ServerStateRequest{})
	if err != nil {
		log.Fatalf("%v.ListFeatures(_) = _, %v", m.grpcClient, err)
	}
	fmt.Println(state)
}
