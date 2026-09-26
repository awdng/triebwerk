package infra

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/awdng/triebwerk-proto/gameserver"
	"github.com/awdng/triebwerk/model"
)

// MasterServerClient ...
type MasterServerClient struct {
	grpcClient pb.GameServerMasterClient
	mutex      sync.RWMutex
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
	m.mutex.Lock()
	m.address = address
	m.mutex.Unlock()
	m.registerServer()
}

// GetServerState ...
func (m *MasterServerClient) GetServerState() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	id, _ := m.identity()
	state, err := m.grpcClient.GetServerState(ctx, &pb.GetServerRequest{
		Id: id,
	})
	if err != nil {
		log.Printf("Error Receiving ServerState: %v", err)
	}
	fmt.Println(state)
}

// RegisterServer ...
func (m *MasterServerClient) registerServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, address := m.identity()
	server, err := m.grpcClient.RegisterServer(ctx, &pb.ServerRegisterRequest{
		Address: address,
	})
	if err != nil {
		log.Printf("Error Registering Server: %v", err)
		return
	}
	m.mutex.Lock()
	m.id = server.Id
	m.mutex.Unlock()
}

// SendHeartbeat ...
func (m *MasterServerClient) SendHeartbeat(snapshot model.StateSnapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := m.grpcClient.SendHeartbeat(ctx, &pb.ServerStateRequest{
		State: m.buildServerState(snapshot),
	})
	if err != nil {
		log.Printf("Error Sending ServerState: %v", err)
	}
}

// EndGame ...
func (m *MasterServerClient) EndGame(snapshot model.StateSnapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := m.grpcClient.EndGame(ctx, &pb.EndGameRequest{
		State: m.buildServerState(snapshot),
	})
	if err != nil {
		log.Printf("Error When Ending Game: %v", err)
	}
}

// AuthorizePlayer returns the global id and nickname of the player owning the token
func (m *MasterServerClient) AuthorizePlayer(token string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	authResp, err := m.grpcClient.AuthorizePlayer(ctx, &pb.AuthorizePlayerRequest{
		Token: token,
	})
	if err != nil {
		log.Printf("Error authorizing player: %v", err)
		// todo wrap error
		return "", "", err
	}

	return authResp.GetGlobalId(), authResp.GetName(), nil
}

func (m *MasterServerClient) identity() (string, string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.id, m.address
}

func (m *MasterServerClient) buildServerState(snapshot model.StateSnapshot) *pb.ServerState {
	players := []*pb.Player{}
	for _, pd := range snapshot.Players {
		p := &pb.Player{
			Name:  pd.Nickname,
			Score: int32(pd.Score),
			Team:  0,
		}
		players = append(players, p)
	}

	id, address := m.identity()
	return &pb.ServerState{
		Region:      snapshot.Region,
		Id:          id,
		Address:     address,
		UpdatedAt:   int32(time.Now().UTC().Unix()),
		ElapsedTime: int32(snapshot.GameTime),
		Players:     players,
	}
}
