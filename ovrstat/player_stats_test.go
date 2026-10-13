package ovrstat

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestSubmatchDoesNotPanic covers the class of bug that used to crash the
// scraper: indexing FindStringSubmatch directly when the markup changed.
func TestSubmatchDoesNotPanic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"matching endorsement icon", "https://cdn.example/icons/3-abc123.svg", "3", true},
		{"no match at all", "https://cdn.example/icons/portrait.png", "", false},
		{"empty string", "", "", false},
		{"partial shape", "/-abc.svg", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := submatch(endorsementRegexp, tt.input, 1)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("submatch(%q) = (%q, %v), want (%q, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseEndorsement(t *testing.T) {
	tests := []struct {
		icon string
		want int
	}{
		{"https://cdn.example/icons/4-deadbeef.svg", 4},
		{"https://cdn.example/icons/5.deadbeef.svg", 5}, // newer dot-separated schema
		{"https://cdn.example/icons/nope.png", 0},
		{"", 0},
	}

	for _, tt := range tests {
		if got := parseEndorsement(tt.icon); got != tt.want {
			t.Errorf("parseEndorsement(%q) = %d, want %d", tt.icon, got, tt.want)
		}
	}
}

func TestRoleFromIcon(t *testing.T) {
	tests := []struct {
		icon string
		want string
	}{
		{"/img/roles/tank-1a2b3c.svg", "tank"},
		{"/img/roles/support-ffff.svg", "support"},
		{"/img/roles/offense-0.svg", "offense"},
		{"/img/roles/tank.1a2b3c.svg", "tank"}, // newer dot-separated schema
		// Previously "role[0:strings.Index(role, \"-\")]" panicked on these.
		{"/img/roles/tank.svg", ""},
		{"", ""},
		{"/img/roles/-leading.svg", ""},
	}

	for _, tt := range tests {
		if got := roleFromIcon(tt.icon); got != tt.want {
			t.Errorf("roleFromIcon(%q) = %q, want %q", tt.icon, got, tt.want)
		}
	}
}

func TestNormalizeEndorsementIcon(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/svg?path=%2Ficons%2F3-a.svg", "/icons/3-a.svg"},
		{"/svg", "/svg"},
		{"https://cdn.example/icons/3-a.svg", "https://cdn.example/icons/3-a.svg"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := normalizeEndorsementIcon(tt.in); got != tt.want {
			t.Errorf("normalizeEndorsementIcon(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCareerURLEscapesTag(t *testing.T) {
	// A tag must never be able to escape the career path or start a query string.
	tests := []struct {
		tag        string
		mustNotHas []string
	}{
		{"../../admin", []string{"/../"}},
		{"foo?x=1", []string{"?"}},
		{"foo#frag", []string{"#"}},
	}

	for _, tt := range tests {
		got := careerURL(tt.tag)
		if !strings.HasPrefix(got, baseURL+"/") {
			t.Errorf("careerURL(%q) = %q, expected it to stay under %q", tt.tag, got, baseURL)
		}
		for _, bad := range tt.mustNotHas {
			if strings.Contains(strings.TrimPrefix(got, baseURL+"/"), bad) {
				t.Errorf("careerURL(%q) = %q, must not contain %q", tt.tag, got, bad)
			}
		}
	}
}

func TestSameUpstreamHost(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"https://overwatch.blizzard.com/en-us/career/x", true},
		{"https://evil.example/en-us/career/x", false},
		{"http://overwatch.blizzard.com/en-us/career/x", false},
		{"https://overwatch.blizzard.com.evil.example/x", false},
	}

	for _, tt := range tests {
		u := mustParseURL(t, tt.raw)
		if got := sameUpstreamHost(u); got != tt.want {
			t.Errorf("sameUpstreamHost(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestFirstNodeOnEmptySelection(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html><body></body></html>"))
	if err != nil {
		t.Fatalf("failed to build document: %v", err)
	}
	if got := firstNode(doc.Find(".does-not-exist")); got != nil {
		t.Fatalf("firstNode on empty selection = %v, want nil", got)
	}
}

func TestBuildRatingSkipsBrokenMarkup(t *testing.T) {
	// A role wrapper with no rank image must be skipped rather than panic or
	// produce a half-filled Rating.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div class="Profile-playerSummary--roleWrapper"></div>`))
	if err != nil {
		t.Fatalf("failed to build document: %v", err)
	}

	sel := doc.Find(".Profile-playerSummary--roleWrapper")
	if _, ok := buildRating(sel, "/img/roles/tank-1a.svg"); ok {
		t.Fatal("buildRating succeeded on markup without a rank icon, want skip")
	}
	if _, ok := buildRating(sel, ""); ok {
		t.Fatal("buildRating succeeded without a role icon, want skip")
	}
}

func TestGameInt(t *testing.T) {
	game := map[string]any{
		"asInt":     42,
		"asFloat":   42.0,
		"asString":  "1,234",
		"asPercent": "55%",
		"garbage":   "n/a",
		"asBool":    true,
	}

	tests := []struct {
		key  string
		want int
		ok   bool
	}{
		{"asInt", 42, true},
		{"asFloat", 42, true},
		{"asString", 1234, true},
		{"asPercent", 55, true},
		{"garbage", 0, false},
		{"asBool", 0, false},
		{"missing", 0, false},
	}

	for _, tt := range tests {
		got, ok := gameInt(game, tt.key)
		if got != tt.want || ok != tt.ok {
			t.Errorf("gameInt(%q) = (%d, %v), want (%d, %v)", tt.key, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCleanJSONKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Games Won", "gamesWon"},
		{"Eliminations per Life", "eliminationsPerLife"},
		{"All Damage Done - Most in Game", "allDamageDoneMostInGame"},
		{"{count, plural, one {Medal} other {Medals}}", "medals"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := cleanJSONKey(tt.in); got != tt.want {
			t.Errorf("cleanJSONKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseType(t *testing.T) {
	if got := parseType("12"); got != 12 {
		t.Errorf("parseType(\"12\") = %v, want int 12", got)
	}
	if got := parseType("1.5"); got != 1.5 {
		t.Errorf("parseType(\"1.5\") = %v, want float 1.5", got)
	}
	if got := parseType("3:21"); got != "3:21" {
		t.Errorf("parseType(\"3:21\") = %v, want string", got)
	}
}

func TestSplitTag(t *testing.T) {
	name, full := splitTag("Player-1234")
	if name != "Player" || full != "Player#1234" {
		t.Errorf("splitTag(\"Player-1234\") = (%q, %q), want (\"Player\", \"Player#1234\")", name, full)
	}

	name, full = splitTag("Player")
	if name != "Player" || full != "Player" {
		t.Errorf("splitTag(\"Player\") = (%q, %q), want (\"Player\", \"Player\")", name, full)
	}
}
