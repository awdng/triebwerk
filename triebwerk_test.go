package triebwerk

import (
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerThatIsToldNothingPlaysRoundsOfTenMinutesWithTwelvePlayers(t *testing.T) {
	// Setenv puts back what was there after the test, Unsetenv is what "told nothing" means
	for _, name := range []string{"GAME_LENGTH", "MAX_PLAYERS"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	var config Config
	require.NoError(t, envconfig.Process("", &config))
	assert.Equal(t, 600, config.GameLength)
	assert.Equal(t, 12, config.MaxPlayers)
}
