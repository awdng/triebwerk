package triebwerk

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerThatIsToldNothingPlaysRoundsOfTenMinutesWithTwelvePlayers(t *testing.T) {
	t.Setenv("GAME_LENGTH", "")
	t.Setenv("MAX_PLAYERS", "")
	var config Config
	require.NoError(t, envconfig.Process("", &config))
	assert.Equal(t, 600, config.GameLength)
	assert.Equal(t, 12, config.MaxPlayers)
}
