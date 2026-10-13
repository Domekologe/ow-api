package main

import (
	"os"

	"github.com/Domekologe/ow-api/service"
)

func main() {
	// Start the service on the configured port.
	service.Start(getenv("PORT", "8080"))
}

// getenv returns an environment variable, falling back to def when it is unset
// or empty.
func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
