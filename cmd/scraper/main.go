package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Domekologe/ow-api/cache"
	"github.com/Domekologe/ow-api/config"
	"github.com/Domekologe/ow-api/ovrstat"
)

func main() {
	log.Println("Starting Overwatch Stats Scraper...")

	// Load configuration
	cfg := config.Load()

	if !cfg.Scraper.Enabled {
		log.Println("Scraper is disabled in configuration. Exiting.")
		return
	}

	if !cfg.Redis.Enabled {
		log.Fatal("Redis must be enabled for scraper to work")
	}

	// Connect to Redis
	redisCache, err := cache.NewRedisCache(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.GetCacheTTL(),
	)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()

	log.Printf("Connected to Redis at %s:%d", cfg.Redis.Host, cfg.Redis.Port)
	log.Printf("Scraper interval: %s", cfg.Scraper.Interval)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create ticker for periodic updates
	interval := cfg.GetScraperInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial scrape
	log.Println("Running initial scrape...")
	scrapeAll(redisCache)

	// Main loop
	for {
		select {
		case <-ticker.C:
			log.Println("Starting scheduled scrape...")
			scrapeAll(redisCache)
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down gracefully...", sig)
			return
		}
	}
}

// scrapeAll fetches and updates all cached players
func scrapeAll(cache *cache.RedisCache) {
	startTime := time.Now()

	// Get all cached player keys (both complete and profile)
	keys, err := cache.GetKeys("ow:stats:*")
	if err != nil {
		log.Printf("Failed to get cached keys: %v", err)
		return
	}

	if len(keys) == 0 {
		log.Println("No cached players found")
		return
	}

	log.Printf("Found %d cached entries to update", len(keys))

	successful := 0
	errors := 0

	for i, key := range keys {
		// Parse key format: ow:stats:platform:tag or ow:stats:platform:tag:profile
		parts := strings.Split(key, ":")
		if len(parts) < 4 {
			log.Printf("Invalid key format: %s", key)
			continue
		}

		platform := parts[2]
		tag := parts[3]
		isProfile := len(parts) == 5 && parts[4] == "profile"

		log.Printf("[%d/%d] Updating %s/%s%s...", i+1, len(keys), platform, tag,
			func() string {
				if isProfile {
					return " (profile)"
				}
				return ""
			}())

		var updateErr error
		if isProfile {
			// Update profile stats
			stats, err := ovrstat.ProfileStats(platform, tag)
			if err != nil {
				log.Printf("  ✗ Failed: %v", err)
				errors++
				updateErr = err
			} else {
				if err := cache.SetProfile(platform, tag, stats); err != nil {
					log.Printf("  ✗ Cache update failed: %v", err)
					errors++
					updateErr = err
				} else {
					log.Printf("  ✓ Updated successfully")
					successful++
				}
			}
		} else {
			// Update complete stats
			stats, err := ovrstat.Stats(platform, tag)
			if err != nil {
				log.Printf("  ✗ Failed: %v", err)
				errors++
				updateErr = err
			} else {
				if err := cache.Set(platform, tag, stats); err != nil {
					log.Printf("  ✗ Cache update failed: %v", err)
					errors++
					updateErr = err
				} else {
					log.Printf("  ✓ Updated successfully")
					successful++
				}
			}
		}

		// Small delay to avoid overwhelming the server
		if updateErr == nil {
			time.Sleep(2 * time.Second)
		}
	}

	duration := time.Since(startTime)
	log.Printf("Scrape completed in %v: %d successful, %d errors",
		duration.Round(time.Second), successful, errors)
}
