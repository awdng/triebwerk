// Command fakemaster is a stand-in for the master server during local
// development and tests. It speaks the part of the master's API that a
// game server uses, accepts every server and every player and keeps
// nothing. The real master server is its own repository.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
)

func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"name": "fakemaster", "guests": true})
	})
	mux.HandleFunc("POST /api/game/servers", func(w http.ResponseWriter, r *http.Request) {
		var server struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&server)
		log.Printf("RegisterServer address=%s name=%q", server.Address, server.Name)
		reply(w, map[string]any{"id": "fake-server-1", "heartbeatSeconds": 5})
	})
	mux.HandleFunc("PUT /api/game/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		var heartbeat struct {
			Online int `json:"online"`
			Round  struct {
				Key            string `json:"key"`
				ElapsedSeconds int    `json:"elapsedSeconds"`
				Ended          bool   `json:"ended"`
			} `json:"round"`
			Players []json.RawMessage `json:"players"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&heartbeat)
		log.Printf("Heartbeat id=%s online=%d round=%s elapsed=%ds ended=%t players=%d", r.PathValue("id"),
			heartbeat.Online, heartbeat.Round.Key, heartbeat.Round.ElapsedSeconds, heartbeat.Round.Ended, len(heartbeat.Players))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/game/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Goodbye id=%s", r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/game/authorize", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Token string `json:"token"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&request)
		if request.Token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			reply(w, map[string]string{"error": "a player needs a name"})
			return
		}
		// a token may be the session of an account on a real master: not for a log
		log.Printf("AuthorizePlayer (a token of %d characters)", len(request.Token))
		reply(w, map[string]any{"playerId": "fake-" + request.Token, "name": request.Token, "guest": false})
	})
	return mux
}

func main() {
	addr := flag.String("addr", ":8081", "http listen address")
	flag.Parse()
	log.Printf("fake master server listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler()))
}
