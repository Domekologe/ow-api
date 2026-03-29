package service

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// NewsType defines the severity of the news item
type NewsType string

const (
	NewsInfo     NewsType = "info"
	NewsWarning  NewsType = "warning"
	NewsCritical NewsType = "critical"
)

// NewsItem represents a single news update
type NewsItem struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Type      NewsType  `json:"type"`
	Timestamp time.Time `json:"timestamp"`
}

// NewsService handles news operations
type NewsService struct {
	filePath string
	mu       sync.RWMutex
	items    []NewsItem
}

var newsService *NewsService

// InitNewsService initializes the news service
func InitNewsService(filePath string) error {
	ns := &NewsService{
		filePath: filePath,
		items:    make([]NewsItem, 0),
	}

	fillFromRootLegacy := false
	data, err := os.ReadFile(filePath)
	primaryExists := err == nil
	if err != nil {
		if os.IsNotExist(err) && filepath.Clean(filePath) != filepath.Clean("news.json") {
			data, err = os.ReadFile("news.json")
			if err == nil {
				fillFromRootLegacy = true
			}
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			newsService = ns
			return nil
		}
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &ns.items); err != nil {
			if filepath.Clean(filePath) == filepath.Clean("news.json") {
				return err
			}
			legacy, err2 := os.ReadFile("news.json")
			if err2 != nil || len(strings.TrimSpace(string(legacy))) == 0 {
				return err
			}
			if err := json.Unmarshal(legacy, &ns.items); err != nil {
				return err
			}
			fillFromRootLegacy = true
		}
	}
	// Primary file existed but is an empty JSON list (common after switching to data/):
	// load legacy ./news.json once and copy into data/news.json.
	if len(ns.items) == 0 && primaryExists && filepath.Clean(filePath) != filepath.Clean("news.json") {
		legacy, err := os.ReadFile("news.json")
		if err == nil && len(strings.TrimSpace(string(legacy))) > 0 {
			var legacyItems []NewsItem
			if json.Unmarshal(legacy, &legacyItems) == nil && len(legacyItems) > 0 {
				ns.items = legacyItems
				fillFromRootLegacy = true
			}
		}
	}
	if fillFromRootLegacy && len(ns.items) > 0 && filepath.Clean(filePath) != filepath.Clean("news.json") {
		b, err := json.MarshalIndent(ns.items, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileReplacing(filePath, b, 0644); err != nil {
			return err
		}
	}

	newsService = ns
	return nil
}

// GetNews returns all news items sorted by timestamp (newest first)
func (s *NewsService) GetNews() []NewsItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]NewsItem, len(s.items))
	copy(result, s.items)

	// Sort by timestamp descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// AddNews adds a new news item
func (s *NewsService) AddNews(content string, newsType NewsType) (NewsItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := NewsItem{
		ID:        uuid.New().String(),
		Content:   content,
		Type:      newsType,
		Timestamp: time.Now(),
	}

	s.items = append(s.items, item)
	if err := s.save(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return NewsItem{}, err
	}
	return item, nil
}

// DeleteNews removes a news item by ID
func (s *NewsService) DeleteNews(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.items {
		if item.ID != id {
			continue
		}
		removed := item
		s.items = append(s.items[:i], s.items[i+1:]...)
		if err := s.save(); err != nil {
			s.items = append(s.items[:i], append([]NewsItem{removed}, s.items[i:]...)...)
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// save persists news items to file
func (s *NewsService) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return writeFileReplacing(s.filePath, data, 0644)
}

// listNews returns all active news
func listNews(c echo.Context) error {
	if newsService == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "News service not initialized",
		})
	}
	return c.JSON(http.StatusOK, newsService.GetNews())
}
