package main

import (
	"log"
	"net/http"

	websocket "github.com/awdng/triebwerk/transport"
)

func main() {
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		websocket.ServeWs(hub, w, r)
	})

	log.Printf("Starting Http Server on Port %s...", "8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
