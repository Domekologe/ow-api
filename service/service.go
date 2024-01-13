package service

import (
	"embed"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
    "github.com/labstack/gommon/log"

    "strings"
)

//go:embed static/*
var staticFS embed.FS

//go:embed docs/*
var docsFS embed.FS

// Start starts serving the service on the passed port
func Start(port string) {
    e := Echo()
    e.Logger.SetLevel(log.DEBUG)
    e.Logger.Info("Server started on Port " + port)
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
    e.Use(middleware.Logger())
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