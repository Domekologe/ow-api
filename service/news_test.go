package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestNewsService(t *testing.T) *NewsService {
	t.Helper()
	return &NewsService{
		filePath: filepath.Join(t.TempDir(), "news.json"),
		items:    make([]NewsItem, 0),
	}
}

func TestNewsAddAndDelete(t *testing.T) {
	s := newTestNewsService(t)

	item, err := s.AddNews("hello", NewsInfo)
	if err != nil {
		t.Fatalf("AddNews failed: %v", err)
	}
	if item.ID == "" {
		t.Fatal("AddNews returned an item without an ID")
	}

	if got := s.GetNews(); len(got) != 1 {
		t.Fatalf("GetNews returned %d items, want 1", len(got))
	}

	ok, err := s.DeleteNews(item.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteNews(%q) = (%v, %v), want (true, nil)", item.ID, ok, err)
	}

	ok, err = s.DeleteNews("does-not-exist")
	if err != nil || ok {
		t.Fatalf("DeleteNews on a missing ID = (%v, %v), want (false, nil)", ok, err)
	}

	if got := s.GetNews(); len(got) != 0 {
		t.Fatalf("GetNews returned %d items after delete, want 0", len(got))
	}
}

func TestNewsGetReturnsCopy(t *testing.T) {
	s := newTestNewsService(t)
	if _, err := s.AddNews("original", NewsInfo); err != nil {
		t.Fatalf("AddNews failed: %v", err)
	}

	got := s.GetNews()
	got[0].Content = "mutated"

	if s.GetNews()[0].Content != "original" {
		t.Fatal("mutating the returned slice changed the service's internal state")
	}
}

func TestNewsSortedNewestFirst(t *testing.T) {
	s := newTestNewsService(t)

	for _, content := range []string{"first", "second", "third"} {
		if _, err := s.AddNews(content, NewsInfo); err != nil {
			t.Fatalf("AddNews failed: %v", err)
		}
	}

	got := s.GetNews()
	for i := 1; i < len(got); i++ {
		if got[i-1].Timestamp.Before(got[i].Timestamp) {
			t.Fatal("GetNews is not sorted newest first")
		}
	}
}

func TestNewsRespectsMaxItems(t *testing.T) {
	s := newTestNewsService(t)

	for i := 0; i < maxNewsItems+25; i++ {
		if _, err := s.AddNews("item", NewsInfo); err != nil {
			t.Fatalf("AddNews failed at %d: %v", i, err)
		}
	}

	if got := len(s.GetNews()); got != maxNewsItems {
		t.Fatalf("news count = %d, want the cap of %d", got, maxNewsItems)
	}
}

func TestNewsPersistsValidJSON(t *testing.T) {
	s := newTestNewsService(t)
	if _, err := s.AddNews("persisted", NewsWarning); err != nil {
		t.Fatalf("AddNews failed: %v", err)
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		t.Fatalf("news file was not written: %v", err)
	}

	var items []NewsItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("persisted file is not valid JSON: %v", err)
	}
	if len(items) != 1 || items[0].Content != "persisted" {
		t.Fatalf("persisted content = %+v, want one item with content %q", items, "persisted")
	}
}

// TestNewsConcurrentAccess is meaningful under -race.
func TestNewsConcurrentAccess(t *testing.T) {
	s := newTestNewsService(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = s.AddNews("concurrent", NewsInfo)
		}()
		go func() {
			defer wg.Done()
			_ = s.GetNews()
		}()
	}
	wg.Wait()
}

func TestNewsTypeValid(t *testing.T) {
	valid := []NewsType{NewsInfo, NewsWarning, NewsCritical}
	for _, nt := range valid {
		if !nt.Valid() {
			t.Errorf("NewsType(%q).Valid() = false, want true", nt)
		}
	}

	invalid := []NewsType{"", "urgent", "INFO", "<script>"}
	for _, nt := range invalid {
		if nt.Valid() {
			t.Errorf("NewsType(%q).Valid() = true, want false", nt)
		}
	}
}
