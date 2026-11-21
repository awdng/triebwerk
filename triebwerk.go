package triebwerk

// Config from Environment Vars
type Config struct {
	// PublicIP is the address or host name clients reach the server at. It
	// is what the master server lists the server under.
	PublicIP string `envconfig:"PUBLIC_IP" required:"false" default:"localhost"`
	// MasterURL of the master server, which lists the servers, knows the
	// players and keeps the scores
	MasterURL string `envconfig:"MASTER_URL" required:"false" default:"http://localhost:8081"`
	// ServerKey is what the master server asks game servers for, if it does
	ServerKey string `envconfig:"SERVER_KEY" required:"false"`
	// ServerName in the list of servers, by default region and address
	ServerName string `envconfig:"SERVER_NAME" required:"false"`
	Region     string `envconfig:"REGION" required:"true" default:"EU"`
	Port       int    `envconfig:"PORT" required:"false" default:"80"`
	MaxPlayers int    `envconfig:"MAX_PLAYERS" required:"false" default:"12"`
	// GameLength of a round in seconds
	GameLength int `envconfig:"GAME_LENGTH" required:"false" default:"300"`
	// TLSCert and TLSKey are the files of a certificate for PublicIP (which
	// is a host name then). With one the server speaks TLS and offers
	// WebTransport next to the WebSocket, on the same port number (UDP).
	TLSCert string `envconfig:"TLS_CERT" required:"false"`
	TLSKey  string `envconfig:"TLS_KEY" required:"false"`
	// WebTransportSelfSigned offers WebTransport with a certificate the
	// server makes itself, for development and tests, see transport.Config
	WebTransportSelfSigned bool `envconfig:"WEBTRANSPORT_SELF_SIGNED" required:"false" default:"false"`
}
