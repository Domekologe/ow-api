package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// setupServices initializes the file-backed services in a temp directory so the
// routes that depend on them can be exercised.
func setupServices(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	if err := InitNewsService(filepath.Join(dir, "news.json")); err != nil {
		t.Fatalf("InitNewsService failed: %v", err)
	}
	if err := InitSeasonResetsService(filepath.Join(dir, "season_resets.json")); err != nil {
		t.Fatalf("InitSeasonResetsService failed: %v", err)
	}
	t.Cleanup(func() {
		newsService = nil
		seasonResetsService = nil
	})
}

// TestPublicRoutesResolve is a regression test for a routing bug where the stats
// endpoints were registered via e.Group(""). A group with an empty prefix makes
// Echo register its own catch-all "/*" not-found route, which shadowed the
// static file handler and turned the landing page into an empty 404.
func TestPublicRoutesResolve(t *testing.T) {
	setupServices(t)
	e := Echo()

	tests := []struct {
		name      string
		path      string
		want      int
		wantBody  bool
		wantCType string
	}{
		{"landing page", "/", http.StatusOK, true, "text/html"},
		{"static asset", "/assets/ovrstat.css", http.StatusOK, true, "text/css"},
		{"docs", "/docs/", http.StatusOK, true, "text/html"},
		{"healthcheck", "/healthcheck", http.StatusOK, false, ""},
		{"news", "/news", http.StatusOK, true, "application/json"},
		{"season resets", "/season-resets", http.StatusOK, true, "application/json"},
		{"admin news page", "/admin/news", http.StatusOK, true, "text/html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("GET %s = %d, want %d (body: %s)", tt.path, rec.Code, tt.want, rec.Body.String())
			}
			if tt.wantBody && rec.Body.Len() == 0 {
				t.Fatalf("GET %s returned an empty body", tt.path)
			}
		})
	}
}

// TestLandingPageServesIndexHTML makes sure "/" really serves the embedded
// index.html and not a directory listing or an error page.
func TestLandingPageServesIndexHTML(t *testing.T) {
	e := Echo()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusOK)
	}

	expected, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("static/index.html is missing from the embedded FS: %v", err)
	}
	if rec.Body.Len() != len(expected) {
		t.Fatalf("GET / returned %d bytes, want %d (the embedded index.html)",
			rec.Body.Len(), len(expected))
	}
}

// TestStatsRoutesAreRegistered inspects the router directly. Serving these paths
// would perform a real upstream request, which does not belong in a unit test.
func TestStatsRoutesAreRegistered(t *testing.T) {
	e := Echo()

	want := map[string]bool{
		"/stats/:platform/:tag/profile":  false,
		"/stats/:platform/:tag/complete": false,
	}

	for _, r := range e.Routes() {
		if _, ok := want[r.Path]; ok && r.Method == http.MethodGet {
			want[r.Path] = true
		}
	}

	for path, found := range want {
		if !found {
			t.Errorf("route GET %s is not registered", path)
		}
	}
}

// TestInvalidParamsRejectedBeforeUpstream confirms validation runs at the edge,
// so a manipulated tag never reaches the scraper. These requests are safe to
// serve precisely because they are rejected before any network call.
func TestInvalidParamsRejectedBeforeUpstream(t *testing.T) {
	e := Echo()

	tests := []struct {
		name string
		path string
	}{
		{"unknown platform", "/stats/xbox/Player-1234/profile"},
		{"path traversal in tag", "/stats/pc/..%2f..%2fadmin/profile"},
		{"query injection in tag", "/stats/pc/player%3Fx%3D1/complete"},
		{"empty-ish tag", "/stats/pc/a/complete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("GET %s = %d, want %d", tt.path, rec.Code, http.StatusBadRequest)
			}
		})
	}
}
