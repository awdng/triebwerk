// Command loadtest connects simulated players to a triebwerk server. Every
// player holds random key combinations for a human-like duration, shoots and
// syncs its clock. A live view of the match is served over HTTP.
//
// With -check the run ends after -d, prints a report per player and exits
// with status 1 if players were dropped, updates stalled or no round ended.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/awdng/triebwerk/loadtest"
	"github.com/awdng/triebwerk/protocol"
)

func main() {
	players := flag.Int("n", 12, "number of simulated players")
	duration := flag.Duration("d", 0, "duration of the test, 0 runs until interrupted")
	reconnect := flag.Int("reconnect", 0, "players that leave once mid-game and rejoin")
	url := flag.String("url", "ws://localhost:9090/echo", "address of the server: ws://host:port/echo for its websocket, https://host:port/wt for WebTransport")
	viewAddr := flag.String("view", ":8090", "address of the live view, empty to disable")
	check := flag.Bool("check", false, "print a report at the end and exit 1 if the server misbehaved")
	minRounds := flag.Int("min-rounds", 1, "with -check: rounds every player has to see end")
	maxGap := flag.Duration("max-gap", 250*time.Millisecond, "with -check: longest allowed pause between state updates")
	minRate := flag.Float64("min-rate", 25, "with -check: state updates per second every player has to receive")
	first := flag.Int("first", 0, "number in the name of the first player (load-00), for bots on several servers")
	tokens := flag.String("tokens", "", "tickets of the master server for this game server, separated by commas: the first players play as the accounts they were made for")
	flag.Parse()

	if *check && *duration == 0 {
		log.Fatal("-check needs a duration (-d)")
	}

	var view *viewer
	runner := loadtest.NewRunner(loadtest.Config{
		URL:         *url,
		Players:     *players,
		FirstName:   *first,
		Tokens:      accountTokens(*tokens),
		Duration:    *duration,
		Reconnect:   *reconnect,
		LogInterval: 10 * time.Second,
		OnState: func(snapshot protocol.SnapshotMessage) {
			if view != nil {
				view.publishState(snapshot)
			}
		},
		OnEvent: func(event string, tick uint32) {
			if view != nil {
				view.publishEvent(event, tick)
			}
		},
	})
	if *viewAddr != "" {
		view = newViewer(runner.Stats, runner.Bots)
		go view.serve(*viewAddr)
	}

	report := runner.Run(context.Background())
	fmt.Print(report.String())

	if !*check {
		return
	}
	errs := report.Check(loadtest.Expectations{
		Players:       *players,
		MinRounds:     *minRounds,
		MaxGap:        *maxGap,
		MinUpdateRate: *minRate,
	})
	for _, err := range errs {
		fmt.Println("FAIL:", err)
	}
	if len(errs) > 0 {
		os.Exit(1)
	}
	fmt.Println("PASS")
}

// accountTokens splits the -tokens flag
func accountTokens(list string) []string {
	var tokens []string
	for _, token := range strings.Split(list, ",") {
		if token = strings.TrimSpace(token); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}
