package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Domekologe/ow-api/cache"
	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/singleflight"
)

var (
	redisCache *cache.RedisCache
	apiTimeout time.Duration

	// softTTL is how long a cached entry is considered fresh. Between softTTL and
	// the hard Redis TTL an entry is still served immediately, but a background
	// refresh is kicked off (stale-while-revalidate).
	softTTL time.Duration

	// scrapeGroup collapses concurrent scrapes for the same player into a single
	// upstream request. Without it, N simultaneous requests for one player produce
	// N requests to Blizzard.
	scrapeGroup singleflight.Group
)

// SetCache wires the shared cache instance and derives the soft TTL from it.
func SetCache(c *cache.RedisCache) {
	redisCache = c
	if c != nil {
		// Refresh in the background once an entry has used up a third of its life.
		softTTL = c.TTL() / 3
	}
}

// scrapeStats fetches full stats, collapsing concurrent identical requests.
func scrapeStats(ctx context.Context, platform, tag string) (*ovrstat.PlayerStats, error) {
	v, err, _ := scrapeGroup.Do("full:"+platform+":"+tag, func() (any, error) {
		return ovrstat.Stats(ctx, platform, tag)
	})
	if err != nil {
		return nil, err
	}
	return v.(*ovrstat.PlayerStats), nil
}

// scrapeProfile fetches profile stats, collapsing concurrent identical requests.
func scrapeProfile(ctx context.Context, platform, tag string) (*ovrstat.PlayerStatsProfile, error) {
	v, err, _ := scrapeGroup.Do("profile:"+platform+":"+tag, func() (any, error) {
		return ovrstat.ProfileStats(ctx, platform, tag)
	})
	if err != nil {
		return nil, err
	}
	return v.(*ovrstat.PlayerStatsProfile), nil
}

// isFresh reports whether a cache entry is young enough to skip a refresh.
func isFresh(cachedAt time.Time) bool {
	return softTTL > 0 && time.Since(cachedAt) < softTTL
}

// requestContext derives a bounded context from the incoming request, so an
// abandoned client connection cancels the upstream scrape too.
func requestContext(c echo.Context) (context.Context, context.CancelFunc) {
	timeout := apiTimeout
	if redisCache == nil {
		// Without a cache there is no fallback, so allow a longer scrape.
		timeout = 30 * time.Second
	}
	return context.WithTimeout(c.Request().Context(), timeout)
}

// mapScrapeError converts a scrape failure into an HTTP response.
func mapScrapeError(platform, tag string, err error) error {
	switch {
	case errors.Is(err, ovrstat.ErrPlayerNotFound):
		logResponse(platform, tag, "player not found")
		return newErr(http.StatusNotFound, "Player not found")
	case errors.Is(err, ovrstat.ErrInvalidPlatform):
		logResponse(platform, tag, "invalid platform")
		return newErr(http.StatusBadRequest, "Invalid platform for this player")
	case errors.Is(err, context.DeadlineExceeded):
		logResponse(platform, tag, "upstream timeout")
		return newErr(http.StatusGatewayTimeout, "Upstream request timed out")
	case errors.Is(err, context.Canceled):
		// The client went away, so there is nothing useful to send.
		return newErr(http.StatusRequestTimeout, "Client closed request")
	default:
		slog.Error("scrape failed", "platform", platform, "tag", tag, "error", err)
		return newErr(http.StatusBadGateway, "Failed to retrieve player stats")
	}
}

// statsComplete serves the full stats payload, cache first.
func statsComplete(c echo.Context) error {
	platform, tag, err := requestParams(c)
	if err != nil {
		return err
	}

	logRequest(platform, tag, c.RealIP())

	// 1. Serve from cache when possible.
	if redisCache != nil {
		entry, cacheErr := redisCache.Get(c.Request().Context(), platform, tag)
		if cacheErr != nil {
			slog.Warn("cache read failed", "platform", platform, "tag", tag, "error", cacheErr)
		} else if entry != nil {
			if isFresh(entry.CachedAt) {
				logResponse(platform, tag, "served from cache")
			} else {
				refreshStatsAsync(platform, tag)
				logResponse(platform, tag, "served stale from cache, refreshing")
			}
			applySeasonResetsIfConfigured(entry.Value)
			return c.JSON(http.StatusOK, entry.Value)
		}
	}

	// 2. Cache miss, so scrape synchronously.
	ctx, cancel := requestContext(c)
	defer cancel()

	stats, err := scrapeStats(ctx, platform, tag)
	if err != nil {
		return mapScrapeError(platform, tag, err)
	}

	storeStats(c.Request().Context(), platform, tag, stats)
	logResponse(platform, tag, "scraped")

	applySeasonResetsIfConfigured(stats)
	return c.JSON(http.StatusOK, stats)
}

// statsProfile serves the reduced profile payload, cache first.
func statsProfile(c echo.Context) error {
	platform, tag, err := requestParams(c)
	if err != nil {
		return err
	}

	logRequest(platform, tag, c.RealIP())

	if redisCache != nil {
		entry, cacheErr := redisCache.GetProfile(c.Request().Context(), platform, tag)
		if cacheErr != nil {
			slog.Warn("profile cache read failed", "platform", platform, "tag", tag, "error", cacheErr)
		} else if entry != nil {
			if isFresh(entry.CachedAt) {
				logResponse(platform, tag, "served profile from cache")
			} else {
				refreshProfileAsync(platform, tag)
				logResponse(platform, tag, "served stale profile from cache, refreshing")
			}
			applySeasonResetsProfileIfConfigured(entry.Value)
			return c.JSON(http.StatusOK, entry.Value)
		}
	}

	ctx, cancel := requestContext(c)
	defer cancel()

	stats, err := scrapeProfile(ctx, platform, tag)
	if err != nil {
		return mapScrapeError(platform, tag, err)
	}

	storeProfile(c.Request().Context(), platform, tag, stats)
	logResponse(platform, tag, "scraped profile")

	applySeasonResetsProfileIfConfigured(stats)
	return c.JSON(http.StatusOK, stats)
}

// storeStats writes stats to the cache, logging but not surfacing failures.
func storeStats(ctx context.Context, platform, tag string, stats *ovrstat.PlayerStats) {
	if redisCache == nil {
		return
	}
	if err := redisCache.Set(ctx, platform, tag, stats); err != nil {
		slog.Warn("cache write failed", "platform", platform, "tag", tag, "error", err)
	}
}

// storeProfile writes profile stats to the cache, logging but not surfacing failures.
func storeProfile(ctx context.Context, platform, tag string, stats *ovrstat.PlayerStatsProfile) {
	if redisCache == nil {
		return
	}
	if err := redisCache.SetProfile(ctx, platform, tag, stats); err != nil {
		slog.Warn("profile cache write failed", "platform", platform, "tag", tag, "error", err)
	}
}
