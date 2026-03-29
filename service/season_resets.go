package service

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/Domekologe/ow-api/seasonmap"
	"github.com/labstack/echo/v4"
)

var seasonResetsService *SeasonResetsService

// SeasonResetsService persists the list of season numbers where the in-game season
// restarts at 1 (scraper season counter keeps increasing).
type SeasonResetsService struct {
	filePath string
	mu       sync.RWMutex
	resets   []int
}

// InitSeasonResetsService loads season reset anchors from disk.
func InitSeasonResetsService(filePath string) error {
	s := &SeasonResetsService{filePath: filePath}
	loaded, err := seasonmap.ReadResetsFile(filePath)
	if err != nil {
		return err
	}
	s.resets = loaded
	seasonResetsService = s
	return nil
}

// Get returns a copy of configured reset season numbers (sorted).
func (s *SeasonResetsService) Get() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]int, len(s.resets))
	copy(out, s.resets)
	return out
}

// Set replaces reset anchors and persists them.
func (s *SeasonResetsService) Set(resets []int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets = seasonmap.NormalizeResets(resets)
	return s.saveUnlocked()
}

func (s *SeasonResetsService) saveUnlocked() error {
	f := seasonmap.ResetsFile{Resets: s.resets}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

func applySeasonResetsIfConfigured(ps *ovrstat.PlayerStats) {
	if ps == nil {
		return
	}
	var anchors []int
	if seasonResetsService != nil {
		anchors = seasonResetsService.Get()
	}
	seasonmap.ApplyPlayerStats(ps, anchors)
}

func applySeasonResetsProfileIfConfigured(ps *ovrstat.PlayerStatsProfile) {
	if ps == nil {
		return
	}
	var anchors []int
	if seasonResetsService != nil {
		anchors = seasonResetsService.Get()
	}
	seasonmap.ApplyPlayerProfile(ps, anchors)
}

// listSeasonResets exposes current anchors for the admin UI (same idea as GET /news).
func listSeasonResets(c echo.Context) error {
	if seasonResetsService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Season resets service not initialized",
		})
	}
	return c.JSON(http.StatusOK, seasonmap.ResetsFile{
		Resets: seasonResetsService.Get(),
	})
}

func adminSaveSeasonResets(c echo.Context) error {
	if seasonResetsService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Season resets service not initialized",
		})
	}

	req := new(seasonmap.ResetsFile)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	if err := seasonResetsService.Set(req.Resets); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to save: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, seasonmap.ResetsFile{
		Resets: seasonResetsService.Get(),
	})
}
