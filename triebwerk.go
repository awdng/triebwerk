package triebwerk

// Config from Environment Vars
type Config struct {
	PublicIP string `envconfig:"PUBLIC_IP" required:"false" default:"localhost"`
}
