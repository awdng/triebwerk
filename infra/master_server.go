// Package infra talks to what is outside of the game server: the master
// server, which keeps the list of game servers, the accounts of the players
// and the scores of every round.
//
// The master speaks JSON over HTTP (its repository documents the API):
//
//	POST   /api/game/servers        this server exists, the answer is its id
//	PUT    /api/game/servers/{id}   heartbeat: who is here, the round, its scores
//	DELETE /api/game/servers/{id}   this server is shutting down
//	POST   /api/game/authorize      who does this token belong to
package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awdng/triebwerk/model"
)

// ErrNotAuthorized is returned for a player the master server turns away
var ErrNotAuthorized = model.ErrNotAuthorized

// errUnknownServer is the master's answer to a server it does not know
var errUnknownServer = errors.New("the master server does not know this server")

// no call to the master takes longer than this
const masterTimeout = 3 * time.Second

// MasterConfig is what the master server is told about this server
type MasterConfig struct {
	// URL of the master server, http://host:port
	URL string
	// Key that game servers have to know, empty if the master asks for none
	Key string
	// Name of the server in the list, by default region and address
	Name   string
	Region string
	Map    string
	// MaxPlayers the server takes
	MaxPlayers int
	// Secure: clients reach the server over TLS (wss, https)
	Secure bool
	// Protocol is the version of the wire protocol to clients
	Protocol int
	// Version of the server's code
	Version string
}

// MasterServerClient ...
type MasterServerClient struct {
	cfg  MasterConfig
	http *http.Client
	// boot tells the rounds of this process from those of the one before it
	boot       string
	retryEvery time.Duration

	// one registration at a time
	registering sync.Mutex
	mutex       sync.RWMutex
	address     string
	id          string
}

// NewMasterServerClient ...
func NewMasterServerClient(cfg MasterConfig) *MasterServerClient {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &MasterServerClient{
		cfg:        cfg,
		http:       &http.Client{Timeout: masterTimeout},
		boot:       strconv.FormatInt(time.Now().UnixMilli(), 36),
		retryEvery: 5 * time.Second,
	}
}

// WaitUntilReachable blocks until the master server answers. A game server
// that can not reach its master could let nobody in.
func (m *MasterServerClient) WaitUntilReachable() {
	for {
		status, err := m.call(http.MethodGet, "/api/config", nil, nil)
		if err == nil && status == http.StatusOK {
			return
		}
		log.Printf("failed to connect to the master server at %s, retrying...", m.cfg.URL)
		time.Sleep(m.retryEvery)
	}
}

// Init registers the server under the address clients reach it at. If the
// master can not be reached now, every heartbeat tries again.
func (m *MasterServerClient) Init(address string) {
	m.mutex.Lock()
	m.address = address
	m.mutex.Unlock()
	m.register("")
}

// register asks the master for this server's id, unless another call has
// done so since the caller looked (it saw the id stale)
func (m *MasterServerClient) register(stale string) string {
	m.registering.Lock()
	defer m.registering.Unlock()
	id, address := m.identity()
	if id != "" && id != stale {
		return id
	}
	name := m.cfg.Name
	if name == "" {
		name = strings.TrimSpace(m.cfg.Region + " " + address)
	}
	var answer struct {
		ID string `json:"id"`
	}
	status, err := m.call(http.MethodPost, "/api/game/servers", map[string]any{
		"name":       name,
		"address":    address,
		"secure":     m.cfg.Secure,
		"region":     m.cfg.Region,
		"map":        m.cfg.Map,
		"maxPlayers": m.cfg.MaxPlayers,
		"protocol":   m.cfg.Protocol,
		"version":    m.cfg.Version,
	}, &answer)
	if err != nil || status != http.StatusOK || answer.ID == "" {
		log.Printf("Error registering with the master server at %s: %s", m.cfg.URL, describe(status, err))
		m.mutex.Lock()
		m.id = ""
		m.mutex.Unlock()
		return ""
	}
	m.mutex.Lock()
	m.id = answer.ID
	m.mutex.Unlock()
	log.Printf("Registered with the master server at %s as %s (%s)", m.cfg.URL, answer.ID, name)
	return answer.ID
}

// SendHeartbeat tells the master that the server is alive, who is on it
// and what the scores of its round are
func (m *MasterServerClient) SendHeartbeat(snapshot model.StateSnapshot) {
	m.report(snapshot)
}

// EndGame reports the final scores of a round. Every heartbeat until the
// next round says the same, so a report that is lost is not lost for good.
func (m *MasterServerClient) EndGame(snapshot model.StateSnapshot) {
	m.report(snapshot)
}

func (m *MasterServerClient) report(snapshot model.StateSnapshot) {
	id, _ := m.identity()
	if id == "" {
		if id = m.register(""); id == "" {
			return
		}
	}
	body := m.heartbeat(snapshot)
	err := m.put(id, body)
	if errors.Is(err, errUnknownServer) {
		// the master was restarted: tell it who we are and say it again
		if id = m.register(id); id == "" {
			return
		}
		err = m.put(id, body)
	}
	if err != nil {
		log.Printf("Error sending the heartbeat to the master server: %s", err)
	}
}

func (m *MasterServerClient) put(id string, body any) error {
	status, err := m.call(http.MethodPut, "/api/game/servers/"+id, body, nil)
	switch {
	case err != nil:
		return err
	case status == http.StatusNotFound:
		return errUnknownServer
	case status != http.StatusNoContent && status != http.StatusOK:
		return errors.New(describe(status, nil))
	}
	return nil
}

type heartbeatPlayer struct {
	Slot          int    `json:"slot"`
	PlayerID      string `json:"playerId"`
	Name          string `json:"name"`
	Kills         int    `json:"kills"`
	Deaths        int    `json:"deaths"`
	SecondsPlayed int    `json:"secondsPlayed"`
	Connected     bool   `json:"connected"`
}

func (m *MasterServerClient) heartbeat(snapshot model.StateSnapshot) map[string]any {
	key := ""
	if snapshot.Round > 0 {
		key = fmt.Sprintf("%s-%d", m.boot, snapshot.Round)
	}
	players := make([]heartbeatPlayer, 0, len(snapshot.Players))
	for _, p := range snapshot.Players {
		players = append(players, heartbeatPlayer{
			Slot: p.Slot, PlayerID: p.GlobalID, Name: p.Nickname, Kills: p.Score, Deaths: p.Deaths,
			SecondsPlayed: p.SecondsPlayed, Connected: p.Connected,
		})
	}
	return map[string]any{
		"online": snapshot.Online,
		"round": map[string]any{
			"key":            key,
			"active":         snapshot.InProgress,
			"elapsedSeconds": int(snapshot.GameTime / 1000),
			"lengthSeconds":  int(snapshot.Length / time.Second),
			"ended":          snapshot.Round > 0 && !snapshot.InProgress,
		},
		"players": players,
	}
}

// AuthorizePlayer returns the account id and the name of the player a token
// belongs to. A guest has a name and no id. ErrNotAuthorized is the master
// saying no; any other error is the master not answering.
func (m *MasterServerClient) AuthorizePlayer(token string) (string, string, error) {
	var answer struct {
		PlayerID string `json:"playerId"`
		Name     string `json:"name"`
	}
	status, err := m.call(http.MethodPost, "/api/game/authorize", map[string]string{"token": token}, &answer)
	switch {
	case err != nil:
		return "", "", fmt.Errorf("the master server did not answer: %w", err)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "", "", ErrNotAuthorized
	case status != http.StatusOK || answer.Name == "":
		return "", "", fmt.Errorf("the master server did not answer: %s", describe(status, nil))
	}
	return answer.PlayerID, answer.Name, nil
}

// Close tells the master that the server is shutting down, so it leaves
// the list at once and not after the heartbeats it will miss
func (m *MasterServerClient) Close() {
	id, _ := m.identity()
	if id == "" {
		return
	}
	if _, err := m.call(http.MethodDelete, "/api/game/servers/"+id, nil, nil); err != nil {
		log.Printf("Error saying goodbye to the master server: %s", err)
	}
}

func (m *MasterServerClient) identity() (string, string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.id, m.address
}

// call sends one request and decodes the answer into answer if there is one
// and the master said yes
func (m *MasterServerClient) call(method, path string, body, answer any) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), masterTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, m.cfg.URL+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if m.cfg.Key != "" {
		request.Header.Set("Authorization", "Bearer "+m.cfg.Key)
	}
	response, err := m.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return response.StatusCode, err
	}
	if answer != nil && response.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, answer); err != nil {
			return response.StatusCode, fmt.Errorf("an answer that is no JSON: %w", err)
		}
	}
	return response.StatusCode, nil
}

func describe(status int, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("it answered with status %d", status)
}
