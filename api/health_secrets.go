package main

import "os"

// healthSecretsFromEnv returns the list of HMAC secrets the /healthz/detailed
// endpoint will accept. The second secret supports a brief rotation window
// where the dashboard still signs with the previous value while services pick
// up the new one. Empty values are dropped by the middleware.
func healthSecretsFromEnv() [][]byte {
	return [][]byte{
		[]byte(os.Getenv("HEALTH_AUTH_SECRET")),
		[]byte(os.Getenv("HEALTH_AUTH_SECRET_PREVIOUS")),
	}
}
