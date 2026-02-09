package service

import (
	"context"
	"net/http"
	"time"

	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

var (
	redisCache *RedisCache
	apiTimeout time.Duration
)

// statsWithTimeout performs a stats lookup with a timeout
func statsWithTimeout(platform, tag string, timeout time.Duration) (*ovrstat.PlayerStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan *ovrstat.PlayerStats, 1)
	errChan := make(chan error, 1)

	go func() {
		stats, err := ovrstat.Stats(platform, tag)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- stats
	}()

	select {
	case stats := <-resultChan:
		return stats, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, errors.New("request timeout")
	}
}

// profileStatsWithTimeout performs a profile stats lookup with a timeout
func profileStatsWithTimeout(platform, tag string, timeout time.Duration) (*ovrstat.PlayerStatsProfile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan *ovrstat.PlayerStatsProfile, 1)
	errChan := make(chan error, 1)

	go func() {
		stats, err := ovrstat.ProfileStats(platform, tag)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- stats
	}()

	select {
	case stats := <-resultChan:
		return stats, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, errors.New("request timeout")
	}
}

// stats handles retrieving and serving Overwatch stats in JSON
func statsComplete(c echo.Context) error {
	platform := c.Param("platform")
	tag := c.Param("tag")
	clientIP := c.RealIP()

	// Log request
	logRequest(platform, tag, clientIP)

	// Try live scraping first with timeout
	stats, err := statsWithTimeout(platform, tag, apiTimeout)

	if err != nil {
		// On timeout, try to use cache data as fallback
		if err.Error() == "request timeout" {
			if redisCache != nil {
				cachedStats, cacheErr := redisCache.Get(platform, tag)
				if cacheErr == nil && cachedStats != nil {
					// Trigger background scraper to refresh
					logResponse(platform, tag, "Timeout - Serving from cache, background scraper triggered")
					triggerScraperUpdate(platform, tag)
					return c.JSON(http.StatusOK, cachedStats)
				}
			}
			logResponse(platform, tag, "Timeout - Sent to background scraper")
			// Trigger scraper even without cache
			triggerScraperUpdate(platform, tag)
			return newErr(http.StatusGatewayTimeout, "Request timeout - Data will be scraped in background")
		}

		// Handle other errors
		if err == ovrstat.ErrPlayerNotFound {
			logResponse(platform, tag, "Player not found")
			return newErr(http.StatusNotFound, "Player not found!")
		}
		logResponse(platform, tag, "Error: "+err.Error())
		return newErr(http.StatusInternalServerError,
			errors.Wrap(err, "Failed to retrieve player stats"))
	}

	// Check if profile is private
	if stats.Private {
		logResponse(platform, tag, "Profile is private")
		// Still cache private profiles
		if redisCache != nil {
			redisCache.Set(platform, tag, stats)
		}
		return c.JSON(http.StatusOK, stats)
	}

	// Store in cache for future requests
	if redisCache != nil {
		if err := redisCache.Set(platform, tag, stats); err == nil {
			logResponse(platform, tag, "Player found - Cached")
		} else {
			logResponse(platform, tag, "Player found - Cache failed")
		}
	} else {
		logResponse(platform, tag, "Player found")
	}

	return c.JSON(http.StatusOK, stats)
}

func statsProfile(c echo.Context) error {
	platform := c.Param("platform")
	tag := c.Param("tag")
	clientIP := c.RealIP()

	// Log request
	logRequest(platform, tag, clientIP)

	// Try live scraping first with timeout
	stats, err := profileStatsWithTimeout(platform, tag, apiTimeout)
	if err != nil {
		// On timeout, try to use cache data as fallback
		if err.Error() == "request timeout" {
			if redisCache != nil {
				cachedStats, cacheErr := redisCache.GetProfile(platform, tag)
				if cacheErr == nil && cachedStats != nil {
					// Trigger background scraper to refresh
					logResponse(platform, tag, "Timeout - Serving from cache (profile), background scraper triggered")
					triggerScraperUpdateProfile(platform, tag)
					return c.JSON(http.StatusOK, cachedStats)
				}
			}
			logResponse(platform, tag, "Timeout - Sent to background scraper (profile)")
			// Trigger scraper even without cache
			triggerScraperUpdateProfile(platform, tag)
			return newErr(http.StatusGatewayTimeout, "Request timeout - Data will be scraped in background")
		}

		// Handle other errors
		if err == ovrstat.ErrPlayerNotFound {
			logResponse(platform, tag, "Player not found")
			return newErr(http.StatusNotFound, "Player not found!")
		}
		logResponse(platform, tag, "Error: "+err.Error())
		return newErr(http.StatusInternalServerError,
			errors.Wrap(err, "Failed to retrieve player stats"))
	}

	// Check if profile is private
	if stats.Private {
		logResponse(platform, tag, "Profile is private")
		// Still cache private profiles
		if redisCache != nil {
			redisCache.SetProfile(platform, tag, stats)
		}
		return c.JSON(http.StatusOK, stats)
	}

	// Store in cache for future requests
	if redisCache != nil {
		if err := redisCache.SetProfile(platform, tag, stats); err == nil {
			logResponse(platform, tag, "Player found (profile) - Cached")
		} else {
			logResponse(platform, tag, "Player found (profile) - Cache failed")
		}
	} else {
		logResponse(platform, tag, "Player found (profile)")
	}

	return c.JSON(http.StatusOK, stats)
}
