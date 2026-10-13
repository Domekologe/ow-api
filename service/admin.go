package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// adminPasswordHash holds the SHA-256 of the configured admin password. Comparing
// fixed-size digests keeps the comparison constant time regardless of input length.
var adminPasswordHash []byte

const (
	// maxNewsContentLength bounds a single news item, so the persisted file and
	// the public /news response cannot grow without limit.
	maxNewsContentLength = 500

	// maxAdminKeyListing bounds how many cache keys the stats endpoint returns.
	maxAdminKeyListing = 500
)

// SetAdminPassword stores the digest of the admin password. An empty password
// disables the admin endpoints entirely.
func SetAdminPassword(password string) {
	password = strings.TrimSpace(password)
	if password == "" {
		adminPasswordHash = nil
		return
	}
	sum := sha256.Sum256([]byte(password))
	adminPasswordHash = sum[:]
}

// adminEnabled reports whether an admin password is configured.
func adminEnabled() bool { return len(adminPasswordHash) > 0 }

// adminAuth authenticates admin requests against the configured password.
func adminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !adminEnabled() {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error": "Admin endpoints are disabled. Set ADMIN_PASSWORD to enable.",
			})
		}

		// Trim the token to tolerate CRLF from .env files and pasted whitespace.
		const prefix = "Bearer "
		auth := strings.TrimSpace(c.Request().Header.Get("Authorization"))
		token := ""
		if after, found := strings.CutPrefix(auth, prefix); found {
			token = strings.TrimSpace(after)
		}

		// Constant-time comparison of digests: a plain != leaks the length of the
		// matching prefix through timing, which makes the password brute-forceable.
		sum := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(sum[:], adminPasswordHash) != 1 {
			slog.Warn("rejected admin request", "ip", c.RealIP(), "path", c.Path())
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Invalid admin password",
			})
		}

		return next(c)
	}
}

// requireCache short-circuits admin handlers that need Redis.
func requireCache(c echo.Context) error {
	if redisCache == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis cache is not enabled",
		})
	}
	return nil
}

// adminFlushCache clears the entire Redis cache.
func adminFlushCache(c echo.Context) error {
	if err := requireCache(c); err != nil {
		return err
	}

	if err := redisCache.FlushAll(c.Request().Context()); err != nil {
		slog.Error("cache flush failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to flush cache",
		})
	}

	slog.Info("cache flushed", "ip", c.RealIP())
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Cache flushed successfully",
	})
}

// adminTriggerScraper reports how many entries the dedicated scraper would refresh.
func adminTriggerScraper(c echo.Context) error {
	if err := requireCache(c); err != nil {
		return err
	}

	count, err := redisCache.CountKeys(c.Request().Context())
	if err != nil {
		slog.Error("cache key count failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to inspect cache",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message":        "Scraper trigger received",
		"note":           "This endpoint only reports state. Actual scraping is done by the dedicated scraper service.",
		"cached_players": count,
	})
}

// adminCacheStats returns cache statistics and a bounded key sample.
func adminCacheStats(c echo.Context) error {
	if err := requireCache(c); err != nil {
		return err
	}

	ctx := c.Request().Context()

	count, err := redisCache.CountKeys(ctx)
	if err != nil {
		slog.Error("cache key count failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get cache stats",
		})
	}

	// Only a bounded sample is returned: a full listing of a large cache would be
	// a multi-megabyte response.
	keys, err := redisCache.Keys(ctx, maxAdminKeyListing)
	if err != nil {
		slog.Error("cache key listing failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get cache stats",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"cached_players": count,
		"cache_keys":     keys,
		"truncated":      count > len(keys),
	})
}

// adminAddNews adds a new news item.
func adminAddNews(c echo.Context) error {
	type addNewsRequest struct {
		Content string   `json:"content"`
		Type    NewsType `json:"type"`
	}

	req := new(addNewsRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Content is required",
		})
	}
	if len([]rune(req.Content)) > maxNewsContentLength {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Content is too long",
		})
	}

	if req.Type == "" {
		req.Type = NewsInfo
	}
	if !req.Type.Valid() {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid news type. Expected 'info', 'warning' or 'critical'.",
		})
	}

	if newsService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "News service not initialized",
		})
	}

	item, err := newsService.AddNews(req.Content, req.Type)
	if err != nil {
		slog.Error("news persist failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to persist news",
		})
	}
	return c.JSON(http.StatusOK, item)
}

// adminDeleteNews removes a news item.
func adminDeleteNews(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "ID is required",
		})
	}

	if newsService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "News service not initialized",
		})
	}

	ok, err := newsService.DeleteNews(id)
	if err != nil {
		slog.Error("news persist failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to persist news",
		})
	}
	if ok {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "News item deleted",
		})
	}

	return c.JSON(http.StatusNotFound, map[string]string{
		"error": "News item not found",
	})
}
