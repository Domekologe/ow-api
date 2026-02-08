package service

import (
	"log"

	"github.com/Domekologe/ow-api/ovrstat"
)

var debugLogging bool

// SetDebugLogging enables or disables debug logging
func SetDebugLogging(enabled bool) {
	debugLogging = enabled
}

// logRequest logs an incoming request in a clean format
func logRequest(platform, tag, ip string) {
	log.Printf("Request: Player %s/%s, from: %s", platform, tag, ip)
}

// logResponse logs the response status
func logResponse(platform, tag string, status string) {
	log.Printf("Response: %s/%s - %s", platform, tag, status)
}

// triggerScraperUpdate adds a player to the scraper queue (async)
func triggerScraperUpdate(platform, tag string) {
	if redisCache == nil {
		return
	}

	go func() {
		// Fetch fresh stats in background
		stats, err := ovrstat.Stats(platform, tag)
		if err != nil {
			log.Printf("Background scraper failed for %s/%s: %v", platform, tag, err)
			return
		}

		// Update cache
		if err := redisCache.Set(platform, tag, stats); err != nil {
			log.Printf("Failed to update cache for %s/%s: %v", platform, tag, err)
			return
		}

		log.Printf("Background scraper updated %s/%s", platform, tag)
	}()
}

// triggerScraperUpdateProfile adds a player profile to the scraper queue (async)
func triggerScraperUpdateProfile(platform, tag string) {
	if redisCache == nil {
		return
	}

	go func() {
		// Fetch fresh profile stats in background
		stats, err := ovrstat.ProfileStats(platform, tag)
		if err != nil {
			log.Printf("Background scraper (profile) failed for %s/%s: %v", platform, tag, err)
			return
		}

		// Update cache
		if err := redisCache.SetProfile(platform, tag, stats); err != nil {
			log.Printf("Failed to update profile cache for %s/%s: %v", platform, tag, err)
			return
		}

		log.Printf("Background scraper updated profile %s/%s", platform, tag)
	}()
}

// getClientIP extracts the real client IP from the request
func getClientIP(c interface{}) string {
	// Type assertion to get echo.Context
	type contextWithRealIP interface {
		RealIP() string
	}

	if ctx, ok := c.(contextWithRealIP); ok {
		return ctx.RealIP()
	}

	return "unknown"
}
