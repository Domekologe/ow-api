package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Domekologe/ow-api/ovrstat"
)

var debugLogging bool

// refreshTimeout bounds a single background refresh.
const refreshTimeout = 45 * time.Second

// refreshSlots caps how many background refreshes may run at once. Without this
// cap a burst of stale cache hits would spawn one goroutine and one upstream
// request per client request, which is a fast route to an upstream ban.
var refreshSlots = make(chan struct{}, 4)

// SetDebugLogging enables or disables debug logging.
func SetDebugLogging(enabled bool) {
	debugLogging = enabled

	level := slog.LevelInfo
	if enabled {
		level = slog.LevelDebug
	}
	slog.SetLogLoggerLevel(level)
	ovrstat.Debug = enabled
}

// logRequest logs an incoming request.
func logRequest(platform, tag, ip string) {
	slog.Info("request", "platform", platform, "tag", tag, "ip", ip)
}

// logResponse logs the outcome of a request.
func logResponse(platform, tag, status string) {
	slog.Info("response", "platform", platform, "tag", tag, "status", status)
}

// goSafely runs fn in a goroutine with panic recovery. Background goroutines are
// not covered by the HTTP recover middleware, so an unguarded panic there would
// take down the whole process.
func goSafely(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered from panic in background task", "task", name, "panic", r)
			}
		}()
		fn()
	}()
}

// acquireRefreshSlot takes a worker slot without blocking. It reports false when
// all slots are busy, in which case the refresh is simply skipped: the entry is
// still within its hard TTL and a later request will retry.
func acquireRefreshSlot() bool {
	select {
	case refreshSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseRefreshSlot() { <-refreshSlots }

// refreshStatsAsync re-scrapes a player's full stats in the background and
// updates the cache. Concurrency is bounded and identical in-flight scrapes are
// collapsed by the singleflight group in scrapeStats.
func refreshStatsAsync(platform, tag string) {
	if redisCache == nil {
		return
	}
	if !acquireRefreshSlot() {
		slog.Debug("skipping background refresh, all slots busy", "platform", platform, "tag", tag)
		return
	}

	goSafely("refresh-stats", func() {
		defer releaseRefreshSlot()

		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()

		stats, err := scrapeStats(ctx, platform, tag)
		if err != nil {
			slog.Warn("background refresh failed", "platform", platform, "tag", tag, "error", err)
			retainEntry(platform, tag, false)
			return
		}
		if err := redisCache.Set(ctx, platform, tag, stats); err != nil {
			slog.Warn("background cache update failed", "platform", platform, "tag", tag, "error", err)
			return
		}
		slog.Info("background refresh completed", "platform", platform, "tag", tag)
	})
}

// retainEntry keeps the existing cache entry alive after a failed refresh. It
// uses its own context because the refresh context may already be expired.
func retainEntry(platform, tag string, isProfile bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisCache.Retain(ctx, platform, tag, isProfile); err != nil {
		slog.Warn("failed to retain cached entry", "platform", platform, "tag", tag, "error", err)
	}
}

// refreshProfileAsync is refreshStatsAsync for the profile-only payload.
func refreshProfileAsync(platform, tag string) {
	if redisCache == nil {
		return
	}
	if !acquireRefreshSlot() {
		slog.Debug("skipping background profile refresh, all slots busy", "platform", platform, "tag", tag)
		return
	}

	goSafely("refresh-profile", func() {
		defer releaseRefreshSlot()

		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		defer cancel()

		stats, err := scrapeProfile(ctx, platform, tag)
		if err != nil {
			slog.Warn("background profile refresh failed", "platform", platform, "tag", tag, "error", err)
			retainEntry(platform, tag, true)
			return
		}
		if err := redisCache.SetProfile(ctx, platform, tag, stats); err != nil {
			slog.Warn("background profile cache update failed", "platform", platform, "tag", tag, "error", err)
			return
		}
		slog.Info("background profile refresh completed", "platform", platform, "tag", tag)
	})
}
