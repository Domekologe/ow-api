package service

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

var adminPassword string

// adminAuth is a middleware that checks for admin password
func adminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if adminPassword == "" {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "Admin endpoints are disabled. Set ADMIN_PASSWORD to enable.",
			})
		}

		// Check Authorization header
		auth := c.Request().Header.Get("Authorization")
		expectedAuth := "Bearer " + adminPassword

		if auth != expectedAuth {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Invalid admin password",
			})
		}

		return next(c)
	}
}

// adminFlushCache clears the entire Redis cache
func adminFlushCache(c echo.Context) error {
	if redisCache == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis cache is not enabled",
		})
	}

	if err := redisCache.FlushAll(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to flush cache: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Cache flushed successfully",
	})
}

// adminTriggerScraper triggers an immediate scraper run
func adminTriggerScraper(c echo.Context) error {
	if redisCache == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis cache is not enabled",
		})
	}

	// Get all cached keys
	keys, err := redisCache.GetKeys("ow:stats:*")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get cache keys: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":        "Scraper trigger received",
		"note":           "This endpoint only triggers the scraper. For actual scraping, use the dedicated scraper service.",
		"cached_players": len(keys),
	})
}

// adminCacheStats returns cache statistics
func adminCacheStats(c echo.Context) error {
	if redisCache == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis cache is not enabled",
		})
	}

	keys, err := redisCache.GetKeys("ow:stats:*")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get cache stats: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"cached_players": len(keys),
		"cache_keys":     keys,
	})
}
