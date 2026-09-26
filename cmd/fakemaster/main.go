// Command fakemaster is a stand-in for the master server during local
// development and load tests. It accepts every server and every player.
package main

import (
	"context"
	"flag"
	"log"
	"net"

	pb "github.com/awdng/triebwerk-proto/gameserver"
	"google.golang.org/grpc"
)

type fakeMaster struct {
	pb.UnimplementedGameServerMasterServer
}

func (*fakeMaster) RegisterServer(_ context.Context, r *pb.ServerRegisterRequest) (*pb.ServerRegisterResponse, error) {
	log.Printf("RegisterServer address=%s", r.Address)
	return &pb.ServerRegisterResponse{Id: "fake-server-1"}, nil
}

func (*fakeMaster) SendHeartbeat(_ context.Context, r *pb.ServerStateRequest) (*pb.ServerState, error) {
	log.Printf("Heartbeat id=%s elapsed=%dms players=%d", r.State.Id, r.State.ElapsedTime, len(r.State.Players))
	return r.State, nil
}

func (*fakeMaster) EndGame(_ context.Context, r *pb.EndGameRequest) (*pb.EndGameResponse, error) {
	log.Printf("EndGame players=%v", r.State.Players)
	return &pb.EndGameResponse{}, nil
}

func (*fakeMaster) AuthorizePlayer(_ context.Context, r *pb.AuthorizePlayerRequest) (*pb.AuthorizePlayerResponse, error) {
	log.Printf("AuthorizePlayer token=%s", r.Token)
	return &pb.AuthorizePlayerResponse{Authorized: true, GlobalId: "fake-" + r.Token, Name: r.Token}, nil
}

func main() {
	addr := flag.String("addr", ":8081", "grpc listen address")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterGameServerMasterServer(server, &fakeMaster{})
	log.Printf("fake master server listening on %s", *addr)
	log.Fatal(server.Serve(listener))
}
