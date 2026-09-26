package triebwerk

// Config from Environment Vars
type Config struct {
	PublicIP         string `envconfig:"PUBLIC_IP" required:"false" default:"localhost"`
	MasterServerGRPC string `envconfig:"MASTERSERVER_GRPC" required:"false" default:"localhost:8081"`
	Region           string `envconfig:"REGION" required:"true" default:"EU"`
	Port             int    `envconfig:"PORT" required:"false" default:"80"`
	MaxPlayers       int    `envconfig:"MAX_PLAYERS" required:"false" default:"12"`
	// GameLength of a round in seconds
	GameLength int `envconfig:"GAME_LENGTH" required:"false" default:"300"`
}
