package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Domekologe/ow-api/cache"
	"github.com/Domekologe/ow-api/config"
	"github.com/Domekologe/ow-api/ovrstat"
)

const (
	// scrapeDelay paces requests against the upstream so a large cache does not
	// look like an attack.
	scrapeDelay = 2 * time.Second

	// perPlayerTimeout bounds a single player refresh.
	perPlayerTimeout = 60 * time.Second
)

func main() {
	slog.Info("starting Overwatch stats scraper")

	cfg := config.Load()

	if cfg.Logging.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		ovrstat.Debug = true
	}

	if !cfg.Scraper.Enabled {
		slog.Info("scraper is disabled in configuration, exiting")
		return
	}

	if !cfg.Redis.Enabled {
		slog.Error("Redis must be enabled for the scraper to work")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	redisCache, err := cache.NewRedisCache(ctx,
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
		cfg.GetCacheTTL(),
	)
	if err != nil {
		slog.Error("failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := redisCache.Close(); err != nil {
			slog.Warn("failed to close Redis connection", "error", err)
		}
	}()

	interval := cfg.GetScraperInterval()
	slog.Info("scraper configured", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("running initial scrape")
	scrapeAll(ctx, redisCache)

	for {
		select {
		case <-ticker.C:
			slog.Info("starting scheduled scrape")
			scrapeAll(ctx, redisCache)
		case <-ctx.Done():
			slog.Info("shutdown signal received, stopping")
			return
		}
	}
}

// scrapeAll refreshes every cached player entry.
func scrapeAll(ctx context.Context, c *cache.RedisCache) {
	// A panic in the scraper parser would otherwise kill the whole process,
	// because this goroutine is not covered by any recover middleware.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("recovered from panic during scrape", "panic", r)
		}
	}()

	startTime := time.Now()

	// Collect the keys first: refreshing while iterating a SCAN cursor would
	// otherwise revisit entries we just rewrote.
	var keys []string
	if err := c.IterKeys(ctx, func(key string) bool {
		keys = append(keys, key)
		return true
	}); err != nil {
		slog.Error("failed to list cached keys", "error", err)
		return
	}

	if len(keys) == 0 {
		slog.Info("no cached players found")
		return
	}

	slog.Info("refreshing cached entries", "count", len(keys))

	successful, failed, skipped := 0, 0, 0

	for i, key := range keys {
		if ctx.Err() != nil {
			slog.Info("scrape interrupted", "processed", i, "total", len(keys))
			return
		}

		platform, tag, isProfile, ok := cache.SplitKey(key)
		if !ok {
			slog.Warn("skipping key with unexpected format", "key", key)
			skipped++
			continue
		}

		slog.Debug("updating entry", "index", i+1, "total", len(keys),
			"platform", platform, "tag", tag, "profile", isProfile)

		if err := refreshEntry(ctx, c, platform, tag, isProfile); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Warn("refresh failed, keeping cached data", "platform", platform, "tag", tag, "profile", isProfile, "error", err)
			if rErr := c.Retain(ctx, platform, tag, isProfile); rErr != nil {
				slog.Warn("failed to retain cached entry", "platform", platform, "tag", tag, "error", rErr)
			}
			failed++
			continue
		}
		successful++

		// Pace requests, but stay responsive to shutdown.
		select {
		case <-time.After(scrapeDelay):
		case <-ctx.Done():
			return
		}
	}

	slog.Info("scrape completed",
		"duration", time.Since(startTime).Round(time.Second),
		"successful", successful,
		"failed", failed,
		"skipped", skipped)
}

// refreshEntry re-scrapes a single cache entry and writes it back.
func refreshEntry(ctx context.Context, c *cache.RedisCache, platform, tag string, isProfile bool) error {
	entryCtx, cancel := context.WithTimeout(ctx, perPlayerTimeout)
	defer cancel()

	if isProfile {
		stats, err := ovrstat.ProfileStats(entryCtx, platform, tag)
		if err != nil {
			return err
		}
		return c.SetProfile(entryCtx, platform, tag, stats)
	}

	stats, err := ovrstat.Stats(entryCtx, platform, tag)
	if err != nil {
		return err
	}
	return c.Set(entryCtx, platform, tag, stats)
}
