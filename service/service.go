package service

import (
	"embed"
	"log"
	"net/http"

	"github.com/Domekologe/ow-api/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	glog "github.com/labstack/gommon/log"

	"strings"
)

//go:embed static/*
var staticFS embed.FS

//go:embed docs/*
var docsFS embed.FS

// Start starts serving the service on the passed port
func Start(port string) {
	// Load configuration
	cfg := config.Load()

	// Initialize Redis if enabled
	if cfg.Redis.Enabled {
		cache, err := NewRedisCache(
			cfg.Redis.Host,
			cfg.Redis.Port,
			cfg.Redis.Password,
			cfg.Redis.DB,
			cfg.GetCacheTTL(),
		)
		if err != nil {
			log.Printf("Warning: Failed to connect to Redis: %v", err)
			log.Printf("Continuing without cache...")
		} else {
			redisCache = cache
			log.Printf("Redis cache enabled (TTL: %s)", cfg.Redis.CacheTTL)
		}
	} else {
		log.Printf("Redis cache disabled")
	}

	// Set API timeout
	apiTimeout = cfg.GetAPITimeout()
	log.Printf("API timeout: %s", cfg.API.Timeout)

	// Set debug logging
	SetDebugLogging(cfg.Logging.Debug)
	if cfg.Logging.Debug {
		log.Printf("Debug logging enabled")
	}

	// Set admin password
	adminPassword = cfg.Admin.Password
	if adminPassword != "" {
		log.Printf("Admin endpoints enabled (password protected)")
	} else {
		log.Printf("Admin endpoints disabled (no password set)")
	}

	e := Echo()
	if !cfg.Logging.Debug {
		// Disable Echo's default logger in production
		e.Logger.SetLevel(glog.OFF)
	} else {
		e.Logger.SetLevel(glog.DEBUG)
	}
	log.Printf("Server started on Port %s", port)
	// Listen on the specified port
	e.Logger.Fatal(e.Start(":" + port))
}

// Echo creates and returns a new echo Echo for the service
func Echo() *echo.Echo {
	// Create a new echo Echo and bind all middleware
	e := echo.New()
	e.HideBanner = true

	// Bind middleware
	e.Pre(customTrailingSlashMiddleware("/docs"))
	// Only enable request logging in debug mode
	if debugLogging {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())
	e.Pre(middleware.Secure())
	e.Use(middleware.Gzip())
	e.Use(middleware.CORS())

	// Serve static content from /static and /docs
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

	// Serve static web content from /docs
	e.GET("/docs", echo.WrapHandler(http.FileServer(http.FS(docsFS))),
		middleware.Rewrite(map[string]string{
			"/docs/": "",
		}),
		middleware.Logger(),
	)
	// Handle stats API requests
	e.GET("/stats/:platform/:tag/profile", statsProfile)
	e.GET("/stats/:platform/:tag/complete", statsComplete)

	// Handle healthcheck requests
	e.GET("/healthcheck", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	// Admin endpoints (password protected)
	admin := e.Group("/admin", adminAuth)
	admin.POST("/cache/flush", adminFlushCache)
	admin.POST("/scraper/trigger", adminTriggerScraper)
	admin.GET("/cache/stats", adminCacheStats)

	return e
}

// customTrailingSlashMiddleware is a custom middleware to remove trailing slashes
// except for specified paths.
func customTrailingSlashMiddleware(excludePaths ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request()
			path := request.URL.Path

			// Check if the path should be excluded from trailing slash removal
			for _, excludePath := range excludePaths {
				if path == excludePath || strings.HasPrefix(path, excludePath+"/") {
					return next(c)
				}
			}

			// Perform trailing slash removal for other paths
			if path != "/" && path[len(path)-1] == '/' {
				newPath := path[:len(path)-1]
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
