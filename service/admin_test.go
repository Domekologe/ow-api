package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// callAdminAuth runs the adminAuth middleware with the given Authorization
// header and reports the resulting status code.
func callAdminAuth(t *testing.T, header string) int {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/cache/flush", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := adminAuth(func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler returned an unexpected error: %v", err)
	}
	return rec.Code
}

func TestAdminAuthDisabledWithoutPassword(t *testing.T) {
	SetAdminPassword("")
	t.Cleanup(func() { SetAdminPassword("") })

	if got := callAdminAuth(t, "Bearer anything"); got != http.StatusForbidden {
		t.Fatalf("status = %d, want %d when no password is configured", got, http.StatusForbidden)
	}
}

func TestAdminAuthAcceptsCorrectPassword(t *testing.T) {
	SetAdminPassword("s3cret")
	t.Cleanup(func() { SetAdminPassword("") })

	if got := callAdminAuth(t, "Bearer s3cret"); got != http.StatusOK {
		t.Fatalf("status = %d, want %d for the correct password", got, http.StatusOK)
	}

	// Whitespace and CRLF from .env files must be tolerated.
	if got := callAdminAuth(t, "Bearer  s3cret \r\n"); got != http.StatusOK {
		t.Fatalf("status = %d, want %d for a padded password", got, http.StatusOK)
	}
}

func TestAdminAuthRejectsBadCredentials(t *testing.T) {
	SetAdminPassword("s3cret")
	t.Cleanup(func() { SetAdminPassword("") })

	bad := []string{
		"",
		"Bearer",
		"Bearer wrong",
		"Bearer s3cre",   // Correct prefix, still rejected.
		"Bearer s3cretX", // Correct prefix plus extra.
		"Basic s3cret",
		"s3cret",
	}

	for _, header := range bad {
		if got := callAdminAuth(t, header); got != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status = %d, want %d", header, got, http.StatusUnauthorized)
		}
	}
}

func TestSetAdminPasswordTrims(t *testing.T) {
	t.Cleanup(func() { SetAdminPassword("") })

	SetAdminPassword("   ")
	if adminEnabled() {
		t.Fatal("a whitespace-only password should leave admin disabled")
	}

	SetAdminPassword(" pw ")
	if !adminEnabled() {
		t.Fatal("a padded password should enable admin")
	}
	if got := callAdminAuth(t, "Bearer pw"); got != http.StatusOK {
		t.Fatalf("status = %d, want %d after trimming", got, http.StatusOK)
	}
}
