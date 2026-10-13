package service

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Domekologe/ow-api/cache"
	"github.com/Domekologe/ow-api/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	glog "github.com/labstack/gommon/log"
	"golang.org/x/time/rate"
)

// limiterRate converts a requests-per-second integer to the rate.Limit the
// Echo middleware expects.
func limiterRate(perSecond int) rate.Limit {
	return rate.Limit(perSecond)
}

//go:embed static/*
var staticFS embed.FS

//go:embed docs/*
var docsFS embed.FS

const (
	// Server timeouts. Without these a client can hold a connection open
	// indefinitely (Slowloris) because Go's defaults are "no timeout".
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 90 * time.Second

	// shutdownGrace is how long in-flight requests may finish on SIGTERM.
	shutdownGrace = 20 * time.Second

	// maxAdminBody bounds admin request bodies.
	maxAdminBody = "64K"

	// Admin rate limiting. The sustained rate is what makes the password
	// impractical to brute-force; the burst exists so the admin UI, which fires
	// several requests when a page loads, is not throttled during normal use.
	adminRateLimit  = 2
	adminRateBurst  = 10
	adminRateExpiry = 3 * time.Minute

	// Public rate limiting for the stats endpoints.
	publicRateLimit  = 20
	publicRateBurst  = 40
	publicRateExpiry = 3 * time.Minute
)

// rateLimiter builds a per-IP rate limiter with an explicit burst allowance.
func rateLimiter(rate, burst int, expiry time.Duration) echo.MiddlewareFunc {
	return middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(
		middleware.RateLimiterMemoryStoreConfig{
			Rate:      limiterRate(rate),
			Burst:     burst,
			ExpiresIn: expiry,
		},
	))
}

// Start boots the service and blocks until the process is asked to shut down.
func Start(port string) {
	cfg := config.Load()

	// Logging has to be configured before anything else logs.
	SetDebugLogging(cfg.Logging.Debug)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.Redis.Enabled {
		c, err := cache.NewRedisCache(ctx,
			cfg.Redis.Host,
			cfg.Redis.Port,
			cfg.Redis.Password,
			cfg.Redis.DB,
			cfg.GetCacheTTL(),
		)
		if err != nil {
			slog.Warn("failed to connect to Redis, continuing without cache", "error", err)
		} else {
			SetCache(c)
			defer func() {
				if err := c.Close(); err != nil {
					slog.Warn("failed to close Redis connection", "error", err)
				}
			}()
			slog.Info("Redis cache enabled", "ttl", cfg.Redis.CacheTTL)
		}
	} else {
		slog.Info("Redis cache disabled")
	}

	apiTimeout = cfg.GetAPITimeout()
	slog.Info("API timeout configured", "timeout", apiTimeout)

	SetAdminPassword(cfg.Admin.Password)
	if adminEnabled() {
		slog.Info("admin endpoints enabled (password protected)")
	} else {
		slog.Warn("admin endpoints disabled (no ADMIN_PASSWORD set)")
	}

	if dir := strings.TrimSpace(cfg.Storage.DataDir); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			slog.Warn("could not create persistence directory", "dir", dir, "error", err)
		}
	}

	newsPath := cfg.NewsJSONPath()
	seasonPath := cfg.SeasonResetsJSONPath()
	if ap, err := filepath.Abs(newsPath); err == nil {
		slog.Info("news persistence file", "path", ap)
	}
	if ap, err := filepath.Abs(seasonPath); err == nil {
		slog.Info("season resets file", "path", ap)
	}

	if err := InitNewsService(newsPath); err != nil {
		slog.Warn("failed to initialize news service", "error", err)
	} else {
		slog.Info("news service initialized")
	}

	if err := InitSeasonResetsService(seasonPath); err != nil {
		slog.Warn("failed to initialize season resets", "error", err)
	} else {
		slog.Info("season resets service initialized")
	}

	e := Echo()
	if cfg.Logging.Debug {
		e.Logger.SetLevel(glog.DEBUG)
	} else {
		e.Logger.SetLevel(glog.OFF)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", port)
		if err := e.StartServer(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections")
	}

	// Graceful shutdown lets in-flight requests and file writes finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		return
	}
	slog.Info("server stopped cleanly")
}

// Echo creates and returns a new echo instance for the service.
func Echo() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Pre-router middleware.
	e.Pre(middleware.Secure())
	e.Pre(customTrailingSlashMiddleware("/docs"))

	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())

	if debugLogging {
		e.Use(middleware.Logger())
	}

	// Avoid compressing admin JSON (some clients mishandle tiny gzip bodies).
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c echo.Context) bool {
			return strings.HasPrefix(c.Request().URL.Path, "/admin")
		},
	}))

	// The public read API is intentionally open to any origin. Admin routes are
	// excluded so a browser on an unrelated site cannot drive them.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		Skipper: func(c echo.Context) bool {
			return strings.HasPrefix(c.Request().URL.Path, "/admin")
		},
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions},
	}))

	// Serve static web content from /static
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(staticFS))),
		middleware.Rewrite(map[string]string{
			"/*": "/static/$1",
		}))
	// Serve static web content from /docs
	e.GET("/docs/*", echo.WrapHandler(http.FileServer(http.FS(docsFS))),
		middleware.Rewrite(map[string]string{
			"/docs/(.*)": "/docs/$1/",
		}))
	e.GET("/docs", echo.WrapHandler(http.FileServer(http.FS(docsFS))),
		middleware.Rewrite(map[string]string{
			"/docs/": "",
		}))

	// Public stats API, rate limited per client IP.
	//
	// The limiter is attached per route rather than via e.Group(""): a group with
	// an empty prefix makes Echo register its own catch-all "/*" not-found route,
	// which shadows the static file handler above and turns "/" into a 404.
	publicLimiter := rateLimiter(publicRateLimit, publicRateBurst, publicRateExpiry)
	e.GET("/stats/:platform/:tag/profile", statsProfile, publicLimiter)
	e.GET("/stats/:platform/:tag/complete", statsComplete, publicLimiter)

	e.GET("/news", listNews)
	e.GET("/season-resets", listSeasonResets)

	e.GET("/healthcheck", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	// Admin endpoints: authenticated, rate limited and body limited.
	admin := e.Group("/admin",
		rateLimiter(adminRateLimit, adminRateBurst, adminRateExpiry),
		middleware.BodyLimit(maxAdminBody),
		adminAuth,
	)
	admin.POST("/cache/flush", adminFlushCache)
	admin.POST("/scraper/trigger", adminTriggerScraper)
	admin.GET("/cache/stats", adminCacheStats)
	admin.POST("/news", adminAddNews)
	admin.DELETE("/news/:id", adminDeleteNews)
	admin.POST("/season-resets", adminSaveSeasonResets)

	// Admin UI pages. These are unauthenticated on purpose: they contain no
	// secrets and prompt for the password client side.
	e.GET("/admin/news", serveEmbeddedPage("static/admin.html"))
	e.GET("/admin/season-reset", serveEmbeddedPage("static/admin-season.html"))

	return e
}

// serveEmbeddedPage returns a handler serving a page from the embedded FS.
func serveEmbeddedPage(name string) echo.HandlerFunc {
	return func(c echo.Context) error {
		data, err := staticFS.ReadFile(name)
		if err != nil {
			return c.String(http.StatusNotFound, "Admin page not found")
		}
		return c.HTMLBlob(http.StatusOK, data)
	}
}

// customTrailingSlashMiddleware removes trailing slashes except for the
// specified paths.
func customTrailingSlashMiddleware(excludePaths ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request()
			path := request.URL.Path

			for _, excludePath := range excludePaths {
				if path == excludePath || strings.HasPrefix(path, excludePath+"/") {
					return next(c)
				}
			}

			if path != "/" && strings.HasSuffix(path, "/") {
				newPath := strings.TrimSuffix(path, "/")
				code := http.StatusMovedPermanently
				if request.Method != http.MethodGet {
					code = http.StatusTemporaryRedirect
				}
				return c.Redirect(code, newPath)
			}

			return next(c)
		}
	}
}
