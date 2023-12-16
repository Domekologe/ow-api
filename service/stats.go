package service

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/pkg/errors"
    "fmt"
)

// stats handles retrieving and serving Overwatch stats in JSON
func statsComplete(c echo.Context) error {
	// Perform a Profile only stats lookup
    stats, err := ovrstat.Stats(c.Param("platform"), c.Param("tag"))
	if err != nil {
    fmt.Println("ERROR")
		if err == ovrstat.ErrPlayerNotFound {
			return newErr(http.StatusNotFound, "Player not found!")
		}
		return newErr(http.StatusInternalServerError,
			errors.Wrap(err, "Failed to retrieve player stats"))
	}
	return c.JSON(http.StatusOK, stats)
}

func statsProfile(c echo.Context) error {
	// Perform a full player stats lookup
	stats, err := ovrstat.ProfileStats(c.Param("platform"), c.Param("tag"))
	if err != nil {
    fmt.Println("ERROR")
		if err == ovrstat.ErrPlayerNotFound {
			return newErr(http.StatusNotFound, "Player not found!")
		}
		return newErr(http.StatusInternalServerError,
			errors.Wrap(err, "Failed to retrieve player stats"))
	}
	return c.JSON(http.StatusOK, stats)
}