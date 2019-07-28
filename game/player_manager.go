package game

import (
	"context"
	"errors"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/model"
)

// PlayerManager ...
type PlayerManager struct {
	firebase *triebwerk.Firebase
}

// NewPlayerManager ...
func NewPlayerManager(firebase *triebwerk.Firebase) *PlayerManager {
	return &PlayerManager{
		firebase: firebase,
	}
}

// Authorize Player
func (p *PlayerManager) Authorize(player *model.Player, token string) error {
	ctx := context.Background()
	client, err := p.firebase.App.Auth(ctx)
	if err != nil {
		return err
	}

	checkedToken, err := client.VerifyIDTokenAndCheckRevoked(ctx, token)
	if err != nil {
		return err
	}
	player.GlobalID = checkedToken.UID

	// user did not verify email
	if emailVerified, ok := checkedToken.Claims["email_verified"]; ok {
		if !emailVerified.(bool) {
			return errors.New("User email not verified")
		}
	}

	return nil
}
