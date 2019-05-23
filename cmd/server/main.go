package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/awdng/triebwerk/game"
	"github.com/awdng/triebwerk/protocol"
)

func main() {
	networkManager := game.NewNetworkManager(protocol.NewBinaryProtocol())
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		networkManager.Serve(w, r)
		fmt.Println("test websocket endpoint called")
	})

	log.Printf("Starting Triebwerk on Port %s...", "8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
