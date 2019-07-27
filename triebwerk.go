package triebwerk

import (
	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
)

// Config from Environment Vars
type Config struct {
	PublicIP string `envconfig:"PUBLIC_IP" required:"false" default:"localhost"`
}

// Firebase ...
type Firebase struct {
	App   *firebase.App
	Store *firestore.Client
}
