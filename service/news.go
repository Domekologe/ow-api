package service

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
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

	// Load existing news if file exists
	if _, err := os.Stat(filePath); err == nil {
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			return err
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &ns.items); err != nil {
				return err
			}
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
func (s *NewsService) AddNews(content string, newsType NewsType) NewsItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := NewsItem{
		ID:        uuid.New().String(),
		Content:   content,
		Type:      newsType,
		Timestamp: time.Now(),
	}

	s.items = append(s.items, item)
	s.save()
	return item
}

// DeleteNews removes a news item by ID
func (s *NewsService) DeleteNews(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, item := range s.items {
		if item.ID == id {
			// Delete item
			s.items = append(s.items[:i], s.items[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// save persists news items to file
func (s *NewsService) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(s.filePath, data, 0644)
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
