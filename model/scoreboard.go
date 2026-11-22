package model

import "time"

// Scoreboard is what clients are told about the players of a round
type Scoreboard struct {
	// RoundLength lets clients show the time that is left
	RoundLength time.Duration
	Players     []ScoreboardEntry
}

// ScoreboardEntry is one player on the scoreboard
type ScoreboardEntry struct {
	ID     int
	Kills  int
	Deaths int
	Name   string
	// Account: the player plays with an account of the master server.
	// Everybody else is a guest.
	Account bool
}

// Kill says who destroyed whom, by player id
type Kill struct {
	Killer int
	Victim int
}

// Scoreboard lists the players with their kills and deaths
func (g *GameState) Scoreboard() *Scoreboard {
	players := g.GetPlayers()
	board := &Scoreboard{
		RoundLength: g.length,
		Players:     make([]ScoreboardEntry, 0, len(players)),
	}
	for _, p := range players {
		board.Players = append(board.Players, ScoreboardEntry{ID: p.ID, Kills: p.Score, Deaths: p.Deaths, Name: p.Nickname, Account: p.GlobalID != ""})
	}
	return board
}
