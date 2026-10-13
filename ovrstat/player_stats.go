package ovrstat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	baseHost = "https://overwatch.blizzard.com"

	baseURL = baseHost + "/en-us/career"

	// API URL Use '#' as A Delimiter between Number and Name and not '-'
	apiURL = baseHost + "/en-us/search/account-by-name/"

	// searchURL is fetched once to obtain the session cookies Blizzard expects.
	searchURL = baseHost + "/en-us/search/"

	// unlocksURL resolves namecard/unlock metadata.
	unlocksURL = baseHost + "/en-us/search/unlocks/"

	// PlatformPC is a platform for PCs (mouseKeyboard in the page)
	PlatformPC = "pc"

	// PlatformConsole is a consolidated platform of all consoles
	PlatformConsole = "console"

	// maxCareerRedirects bounds the redirect chain followed when resolving a career ID.
	maxCareerRedirects = 5

	// upstreamTimeout bounds a single upstream HTTP request.
	upstreamTimeout = 15 * time.Second

	// sessionTTL is how long a primed session is reused before being refreshed.
	sessionTTL = 30 * time.Minute

	// maxResponseBytes bounds how much of an upstream response we are willing to
	// read, so a hostile or broken upstream cannot exhaust memory.
	maxResponseBytes = 16 << 20 // 16 MiB
)

var (
	// ErrPlayerNotFound is returned when a player doesn't exist.
	ErrPlayerNotFound = errors.New("player not found")

	// ErrInvalidPlatform is returned when the requested platform has no profile view.
	ErrInvalidPlatform = errors.New("invalid platform")

	// ErrUnexpectedMarkup is returned when the upstream page no longer matches the
	// structure this scraper expects. It signals that the scraper needs updating
	// rather than that the caller did something wrong.
	ErrUnexpectedMarkup = errors.New("unexpected upstream markup")

	// Debug enables verbose debug logging.
	Debug = false
)

// owClientOnce guards lazy construction of the shared HTTP client. The client is
// deliberately process-wide so connections and the cookie jar are reused.
var (
	owClientOnce sync.Once
	owClient     *http.Client

	// redirectClient shares the transport but never follows redirects, because
	// resolveCareerID needs to inspect Location headers itself.
	redirectClientOnce sync.Once
	redirectClient     *http.Client

	sharedTransport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
)

func getOWClient() *http.Client {
	owClientOnce.Do(func() {
		jar, _ := cookiejar.New(nil)
		owClient = &http.Client{
			Timeout:   upstreamTimeout,
			Jar:       jar,
			Transport: sharedTransport,
		}
		debugf("http.Client initialised with cookie jar")
	})
	return owClient
}

func getRedirectClient() *http.Client {
	redirectClientOnce.Do(func() {
		jar, _ := cookiejar.New(nil)
		redirectClient = &http.Client{
			Timeout:   upstreamTimeout,
			Jar:       jar,
			Transport: sharedTransport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	})
	return redirectClient
}

// debugf logs only when Debug is enabled.
func debugf(format string, args ...any) {
	if Debug {
		slog.Debug("ovrstat: " + fmt.Sprintf(format, args...))
	}
}

// submatch returns capture group n of the first match, or "" plus false if the
// pattern did not match. It replaces direct indexing of FindStringSubmatch,
// which panics as soon as the upstream markup changes.
func submatch(re *regexp.Regexp, s string, n int) (string, bool) {
	m := re.FindStringSubmatch(s)
	if len(m) <= n {
		return "", false
	}
	return m[n], true
}

// firstNode returns the first node of a selection, or nil when the selection is empty.
func firstNode(sel *goquery.Selection) *html.Node {
	if len(sel.Nodes) == 0 {
		return nil
	}
	return sel.Nodes[0]
}

// sessionState tracks when the shared cookie jar was last primed, so we do not
// pay an extra round trip on every single upstream call.
var sessionState struct {
	mu       sync.Mutex
	primedAt time.Time
}

// ensureOWSession primes the shared session at most once per sessionTTL.
func ensureOWSession(ctx context.Context, c *http.Client) error {
	sessionState.mu.Lock()
	defer sessionState.mu.Unlock()

	if !sessionState.primedAt.IsZero() && time.Since(sessionState.primedAt) < sessionTTL {
		return nil
	}
	if err := primeOWSession(ctx, c); err != nil {
		return err
	}
	sessionState.primedAt = time.Now()
	return nil
}

// invalidateOWSession forces the next ensureOWSession call to re-prime.
func invalidateOWSession() {
	sessionState.mu.Lock()
	sessionState.primedAt = time.Time{}
	sessionState.mu.Unlock()
}

func primeOWSession(ctx context.Context, c *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return fmt.Errorf("prime build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ovrstat)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	debugf("prime request url=%s", searchURL)

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("prime do: %w", err)
	}
	defer resp.Body.Close()

	debugf("prime response status=%d", resp.StatusCode)

	// Drain the body so the connection can be reused, but bound how much we read.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prime session failed: %d %s", resp.StatusCode, resp.Status)
	}
	return nil
}

func splitTag(tag string) (name string, full string) {
	full = strings.ReplaceAll(tag, "-", "#")
	if idx := strings.Index(full, "#"); idx != -1 {
		name = full[:idx]
	} else {
		name = full
	}
	return
}

func resolvePlayerByDoubleSearch(ctx context.Context, tag string) (*Player, error) {
	// 1. Verify the player exists via the career redirect.
	if _, err := resolveCareerID(ctx, tag); err != nil {
		// Pass the error through: only a real miss may surface as ErrPlayerNotFound.
		return nil, err
	}

	// 2. Extract the plain name.
	name, full := splitTag(tag)

	// 3. The search endpoint is only used as a metadata source from here on.
	playersByName, _ := retrievePlayers(ctx, name)
	playersByFull, _ := retrievePlayers(ctx, full)

	// 4. Prefer an exact full-tag hit.
	if len(playersByFull) > 0 {
		return &playersByFull[0], nil
	}

	// 5. Fall back to a name search that yielded exactly one candidate.
	if len(playersByName) == 1 {
		return &playersByName[0], nil
	}

	// 6. The player exists but could not be identified unambiguously.
	return &Player{
		BattleTag: strings.ReplaceAll(tag, "-", "#"),
		IsPublic:  true, // unknown, so assume public
	}, nil
}

// GetUnlockInfo resolves unlock (namecard) metadata for the given unlock ID.
func GetUnlockInfo(ctx context.Context, unlockID string) (*UnlockData, error) {
	c := getOWClient()

	if err := ensureOWSession(ctx, c); err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("unlockIds", unlockID)
	fullURL := unlocksURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")
	req.Header.Set("Referer", searchURL)
	req.Header.Set("Accept-Language", "en,de-DE;q=0.9,de;q=0.8,en-US;q=0.7")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"135\", \"Not-A.Brand\";v=\"99\"")
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", "\"Windows\"")

	debugf("unlock request url=%s", fullURL)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	debugf("unlock response status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		// A rejected request usually means our session cookies went stale.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			invalidateOWSession()
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		debugf("unlock response body=%s", string(b))
		return nil, fmt.Errorf("unlocks %d: %s", resp.StatusCode, string(b))
	}

	var unlocks []UnlockData
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&unlocks); err != nil {
		return nil, err
	}

	debugf("decoded unlocks count=%d", len(unlocks))

	for _, u := range unlocks {
		if u.ID == unlockID {
			return &u, nil
		}
	}

	debugf("unlock not found id=%s", unlockID)
	return nil, fmt.Errorf("unlock ID %q not found", unlockID)
}

// careerURL builds the career URL for a tag. The tag is path-escaped so a hostile
// value cannot traverse to a different upstream path or inject a query string.
func careerURL(tag string) string {
	return baseURL + "/" + url.PathEscape(tag)
}

// sameUpstreamHost reports whether u stays on the Blizzard host we scrape. It is
// the guard that keeps a manipulated redirect from turning into an SSRF.
func sameUpstreamHost(u *url.URL) bool {
	return u.Scheme == "https" && (u.Host == "overwatch.blizzard.com")
}

func resolveCareerID(ctx context.Context, tag string) (string, error) {
	tag = strings.ReplaceAll(tag, "#", "-")

	target := careerURL(tag)
	debugf("resolveCareerID tag=%s url=%s", tag, target)

	client := getRedirectClient()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	setCareerHeaders(req)

	for i := 0; i < maxCareerRedirects; i++ {
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		debugf("redirect step=%d status=%d location=%s", i, resp.StatusCode, resp.Header.Get("Location"))

		// Case 1: a redirect we have to follow manually.
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				drainAndClose(resp)
				return "", errors.New("redirect without location")
			}

			decodedLoc, _ := url.PathUnescape(loc)

			if strings.Contains(decodedLoc, "|") {
				id := strings.Trim(strings.TrimPrefix(decodedLoc, "/career/"), "/")
				drainAndClose(resp)
				return id, nil
			}

			next, err := resp.Request.URL.Parse(loc)
			drainAndClose(resp)
			if err != nil {
				return "", fmt.Errorf("invalid redirect location: %w", err)
			}
			if !sameUpstreamHost(next) {
				return "", fmt.Errorf("refusing cross-host redirect to %q", next.Host)
			}

			req, err = http.NewRequestWithContext(ctx, http.MethodGet, next.String(), nil)
			if err != nil {
				return "", err
			}
			setCareerHeaders(req)
			continue
		}

		// Case 2: a final 200 response, so evaluate the resulting URL.
		if resp.StatusCode == http.StatusOK {
			decodedPath, _ := url.PathUnescape(resp.Request.URL.Path)

			if strings.Contains(decodedPath, "|") {
				id := strings.Trim(strings.TrimPrefix(decodedPath, "/en-us/career/"), "/")
				drainAndClose(resp)
				return id, nil
			}
		}

		status := resp.StatusCode
		drainAndClose(resp)
		// Rate limits and outages must not be reported as a missing player.
		if status != http.StatusOK && status != http.StatusNotFound {
			return "", fmt.Errorf("career lookup returned %d", status)
		}
		break
	}

	debugf("resolveCareerID failed tag=%s", tag)

	return "", ErrPlayerNotFound
}

func setCareerHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ovrstat)")
	req.Header.Set("Accept", "text/html")
}

// drainAndClose consumes a bounded amount of the body before closing, so the
// underlying connection can be returned to the idle pool.
func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// mapPlatformKey translates the public platform name to the identifier the page uses.
func mapPlatformKey(platformKey string) string {
	switch platformKey {
	case PlatformPC:
		return "mouseKeyboard"
	case PlatformConsole:
		return "controller"
	}
	return platformKey
}

// fetchProfileDocument resolves the career ID for a tag and loads the parsed
// profile page. It is shared by Stats and ProfileStats.
func fetchProfileDocument(ctx context.Context, tag string) (*goquery.Document, error) {
	careerID, err := resolveCareerID(ctx, tag)
	if err != nil {
		return nil, err
	}

	profileURL := baseURL + "/" + careerID + "/"
	debugf("resolved careerID=%s profileURL=%s", careerID, profileURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, err
	}
	setCareerHeaders(req)

	res, err := getOWClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve profile: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile request returned %d", res.StatusCode)
	}

	pd, err := goquery.NewDocumentFromReader(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create goquery document: %w", err)
	}

	// The site still answers 200 for unknown players, so detect it by content.
	if pd.Find("[slot=heading]").First().Text() == "Page Not Found" {
		return nil, ErrPlayerNotFound
	}

	return pd, nil
}

// collectPlatforms builds the per-platform view map from the profile document.
func collectPlatforms(pd *goquery.Document) map[string]Platform {
	platforms := make(map[string]Platform)

	pd.Find(".Profile-player--filters .Profile-player--filter").Each(func(i int, sel *goquery.Selection) {
		rawID, _ := sel.Attr("id")

		id, ok := submatch(filterRegexp, rawID, 1)
		if !ok {
			debugf("skipping filter with unexpected id=%q", rawID)
			return
		}

		viewID := "." + id + "-view"

		// Using combined classes (.class.class2) we can filter views by platform.
		rankWrapper := pd.Find(".Profile-playerSummary--rankWrapper" + viewID)
		view := pd.Find(".Profile-view" + viewID)

		if view.Length() == 0 {
			return
		}

		platforms[id] = Platform{
			Name:        sel.Text(),
			RankWrapper: rankWrapper,
			ProfileView: view,
		}
	})

	return platforms
}

// Stats retrieves the full player stats for the given platform and tag.
func Stats(ctx context.Context, platformKey, tag string) (*PlayerStats, error) {
	platformKey = mapPlatformKey(platformKey)

	var ps PlayerStats

	player, err := resolvePlayerByDoubleSearch(ctx, tag)
	if err != nil {
		return nil, err
	}

	if !player.IsPublic {
		ps.Private = true
		return &ps, nil
	}

	ps.NamecardImage = player.Namecard

	pd, err := fetchProfileDocument(ctx, tag)
	if err != nil {
		return nil, err
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	platform, exists := collectPlatforms(pd)[platformKey]
	if !exists {
		return nil, ErrInvalidPlatform
	}

	// Scrapes all stats for the passed user and sets struct member data
	parseGeneralInfo(ctx, platform, pd.Find(".Profile-masthead").First(), &ps)

	parseDetailedStats(platform, ".quickPlay-view", &ps.QuickPlayStats.StatsCollection)
	parseDetailedStats(platform, ".competitive-view", &ps.CompetitiveStats.StatsCollection)

	if seasonAttr, _ := pd.Find("[data-latestherostatrankseasonow2]").Attr("data-latestherostatrankseasonow2"); seasonAttr != "" {
		if season, err := strconv.Atoi(seasonAttr); err == nil {
			ps.CompetitiveStats.Season = &season
		} else {
			debugf("unparsable competitive season attr=%q", seasonAttr)
		}
	}

	addGameStats(&ps, &ps.QuickPlayStats.StatsCollection)
	addGameStats(&ps, &ps.CompetitiveStats.StatsCollection)

	return &ps, nil
}

// gameInt reads an int stat from a parsed game map. Values are produced by
// parseType and may be int, float64 or string, so never assert the type directly.
func gameInt(game map[string]any, key string) (int, bool) {
	v, ok := game[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case string:
		i, err := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(val, ",", ""), "%", ""))
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// gameString reads a string stat from a parsed game map.
func gameString(game map[string]any, key string) (string, bool) {
	v, ok := game[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func addGameStats(ps *PlayerStats, statsCollection *StatsCollection) {
	heroStats, ok := statsCollection.CareerStats["allHeroes"]
	if !ok {
		return
	}
	if v, ok := gameInt(heroStats.Game, "gamesPlayed"); ok {
		ps.GamesPlayed += v
	}
	if v, ok := gameInt(heroStats.Game, "gamesWon"); ok {
		ps.GamesWon += v
	}
	if v, ok := gameInt(heroStats.Game, "gamesLost"); ok {
		ps.GamesLost += v
	}
}

// ProfileStats retrieves the reduced profile-only stats. This is primarily used
// by the OWidget 2 application.
func ProfileStats(ctx context.Context, platformKey, tag string) (*PlayerStatsProfile, error) {
	platformKey = mapPlatformKey(platformKey)

	var ps PlayerStatsProfile

	player, err := resolvePlayerByDoubleSearch(ctx, tag)
	if err != nil {
		return nil, err
	}

	if !player.IsPublic {
		ps.Private = true
		return &ps, nil
	}

	ps.NamecardImage = player.Namecard

	pd, err := fetchProfileDocument(ctx, tag)
	if err != nil {
		return nil, err
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	platform, exists := collectPlatforms(pd)[platformKey]
	if !exists {
		return nil, ErrInvalidPlatform
	}

	// Scrapes all stats for the passed user and sets struct member data
	parseGeneralInfoProfile(ctx, platform, pd.Find(".Profile-masthead").First(), &ps)

	careerStats := parseCareerStats(platform.ProfileView.Find(".stats.competitive-view"))
	fillSummaryTotals(&ps.CompetitiveStats.SummaryStats, careerStats)

	careerStatsQP := parseCareerStats(platform.ProfileView.Find(".stats.quickPlay-view"))
	fillSummaryTotals(&ps.QuickplayStats.SummaryStats, careerStatsQP)

	if seasonAttr, exists := pd.Find("[data-latestherostatrankseasonow2]").Attr("data-latestherostatrankseasonow2"); exists {
		if seasonNumber, err := strconv.Atoi(seasonAttr); err == nil {
			ps.CompetitiveStats.Season = &seasonNumber
		} else {
			debugf("unparsable competitive season attr=%q", seasonAttr)
		}
	}

	fillMostPlayedHero(&ps.CompetitiveStats.SummaryStats, platform, ".competitive-view", careerStats)
	fillMostPlayedHero(&ps.QuickplayStats.SummaryStats, platform, ".quickPlay-view", careerStatsQP)

	return &ps, nil
}

// fillSummaryTotals copies the aggregated "allHeroes" totals into a summary.
func fillSummaryTotals(sum *SummaryStats, careerStats map[string]*CareerStats) {
	heroStats, ok := careerStats["allHeroes"]
	if !ok {
		return
	}
	if v, ok := gameInt(heroStats.Game, "gamesPlayed"); ok {
		sum.GamesPlayed = v
	}
	if v, ok := gameInt(heroStats.Game, "gamesWon"); ok {
		sum.GamesWon = v
	}
	if v, ok := gameInt(heroStats.Game, "gamesLost"); ok {
		sum.GamesLost = v
	}
	if v, ok := gameString(heroStats.Game, "timePlayed"); ok {
		sum.TimePlayed = v
	}
}

// fillMostPlayedHero resolves the most played hero for a play mode and copies
// its games played / win percentage into the summary.
func fillMostPlayedHero(sum *SummaryStats, platform Platform, playMode string, careerStats map[string]*CareerStats) {
	view := platform.ProfileView.Find(".Profile-heroSummary--view" + playMode)

	sum.MostPlayedHero = strings.TrimSpace(view.Find(".Profile-progressBar-title").First().Text())
	sum.MostPlayedHeroTimePlayed = view.Find(".Profile-progressBar-description").First().Text()

	heroStats, ok := careerStats[cleanJSONKey(sum.MostPlayedHero)]
	if !ok {
		return
	}
	if v, ok := gameInt(heroStats.Game, "gamesPlayed"); ok {
		sum.MostPlayedHeroGamesPlayed = v
	}
	if v, ok := gameInt(heroStats.Game, "winPercentage"); ok {
		sum.MostPlayedHeroWinPercentage = v
	}
}

func retrievePlayers(ctx context.Context, tag string) ([]Player, error) {
	tag = strings.ReplaceAll(tag, "-", "#")

	// The tag is user supplied, so escape it instead of concatenating it raw.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+url.PathEscape(tag), nil)
	if err != nil {
		return nil, err
	}
	setCareerHeaders(req)

	apires, err := getOWClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform platform API request: %w", err)
	}
	defer apires.Body.Close()

	if apires.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform API returned %d", apires.StatusCode)
	}

	var platforms []Player
	if err := json.NewDecoder(io.LimitReader(apires.Body, maxResponseBytes)).Decode(&platforms); err != nil {
		return nil, fmt.Errorf("failed to decode platform API response: %w", err)
	}
	return platforms, nil
}

var (
	endorsementRegexp = regexp.MustCompile("/(\\d+)[.-]([a-z0-9]+)\\.svg")
	rankRegexp        = regexp.MustCompile(`https://\S+Rank_([a-zA-Z]+)Tier[.-]([a-f0-9]+)\.png`)
	tierRegexp        = regexp.MustCompile(`https://\S+TierDivision_(\d+)[.-][a-f0-9]+\.png`)
	filterRegexp      = regexp.MustCompile("^([a-zA-Z]+)Filter$")
)

// populateGeneralInfo extracts the users general info and returns it in a
// PlayerStats struct

// parseEndorsement extracts the endorsement level from its icon URL. A markup
// change makes this return 0 rather than panicking.
func parseEndorsement(icon string) int {
	raw, ok := submatch(endorsementRegexp, icon, 1)
	if !ok {
		debugf("endorsement icon did not match: %q", icon)
		return 0
	}
	level, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return level
}

// normalizeEndorsementIcon unwraps the /svg?path= indirection used for some icons.
func normalizeEndorsementIcon(icon string) string {
	if !strings.HasPrefix(icon, "/svg") {
		return icon
	}
	idx := strings.Index(icon, "?")
	if idx < 0 {
		return icon
	}
	q, err := url.ParseQuery(icon[idx+1:])
	if err != nil || q.Get("path") == "" {
		return icon
	}
	return q.Get("path")
}

// roleFromIcon extracts the role from an icon path of the form
// /(offense|support|tank)-HEX.svg (newer pages use '.' instead of '-').
// Returns "" when the shape does not match.
func roleFromIcon(roleIcon string) string {
	if roleIcon == "" {
		return ""
	}
	base := strings.TrimSuffix(path.Base(roleIcon), path.Ext(roleIcon))
	idx := strings.IndexAny(base, ".-")
	if idx <= 0 {
		debugf("role icon has unexpected shape: %q", roleIcon)
		return ""
	}
	return base[:idx]
}

// attrValue returns the value of the named attribute on a node.
func attrValue(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// buildRating assembles a Rating from a role wrapper selection. It reports false
// when the markup does not contain the pieces we need, so the caller can skip
// the entry instead of emitting bogus data.
func buildRating(sel *goquery.Selection, roleIcon string) (Rating, bool) {
	role := roleFromIcon(roleIcon)
	if role == "" {
		return Rating{}, false
	}

	rankIcon, _ := sel.Find("img.Profile-playerSummary--rank").Attr("src")
	tierIcon, _ := sel.Find("img.Profile-playerSummary--rank").Eq(1).Attr("src")

	group, ok := submatch(rankRegexp, rankIcon, 1)
	if !ok {
		debugf("rank icon did not match: %q", rankIcon)
		return Rating{}, false
	}

	tier := 0
	if tierRaw, ok := submatch(tierRegexp, tierIcon, 1); ok {
		tier, _ = strconv.Atoi(tierRaw)
	} else {
		debugf("tier icon did not match: %q", tierIcon)
	}

	return Rating{
		Group:    group,
		Tier:     tier,
		Role:     role,
		RoleIcon: roleIcon,
		RankIcon: rankIcon,
		TierIcon: tierIcon,
	}, true
}

// resolveNamecard looks up namecard metadata and returns title and image.
func resolveNamecard(ctx context.Context, namecardID string) (title, image string) {
	unlockInfo, err := GetUnlockInfo(ctx, namecardID)
	if err != nil {
		debugf("namecard lookup failed id=%s: %v", namecardID, err)
		return "", ""
	}
	return unlockInfo.Name, unlockInfo.Icon
}

// roleIconFromWrapper reads the role icon out of a role wrapper, handling both
// the <img> and the <svg><use> variants Blizzard has shipped.
func roleIconFromWrapper(sel *goquery.Selection) string {
	roleElement := firstNode(sel.Find(".Profile-playerSummary--role"))
	if roleElement == nil {
		return ""
	}

	switch {
	case roleElement.Data == "div":
		return attrValue(firstNode(sel.Find(".Profile-playerSummary--role img")), "src")
	case roleElement.Namespace == "svg":
		return attrValue(firstNode(sel.Find(".Profile-playerSummary--role use")), "href")
	}
	return ""
}

func parseGeneralInfo(ctx context.Context, platform Platform, s *goquery.Selection, ps *PlayerStats) {
	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement = parseEndorsement(ps.EndorsementIcon)
	ps.Title = s.Find(".Profile-player--title").Text()

	if namecardAttr, exists := s.Attr("namecard-id"); exists {
		ps.NamecardID = namecardAttr
		if title, image := resolveNamecard(ctx, namecardAttr); title != "" || image != "" {
			ps.NamecardTitle = title
			ps.NamecardImage = image
		}
	}

	ps.EndorsementIcon = normalizeEndorsementIcon(ps.EndorsementIcon)

	// Ratings. Note that .is-active is the default platform.
	platform.RankWrapper.Find(".Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		if rating, ok := buildRating(sel, roleIconFromWrapper(sel)); ok {
			ps.Ratings = append(ps.Ratings, rating)
		}
	})
}

func parseGeneralInfoProfile(ctx context.Context, platform Platform, s *goquery.Selection, ps *PlayerStatsProfile) {
	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement = parseEndorsement(ps.EndorsementIcon)
	ps.Title = s.Find(".Profile-player--title").Text()

	if namecardAttr, exists := s.Attr("namecard-id"); exists {
		ps.NamecardID = namecardAttr
		if title, image := resolveNamecard(ctx, namecardAttr); title != "" || image != "" {
			ps.NamecardTitle = title
			ps.NamecardImage = image
		}
	}

	ps.EndorsementIcon = normalizeEndorsementIcon(ps.EndorsementIcon)

	// Ratings. Note that .is-active is the default platform.
	platform.RankWrapper.Find("div.Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		roleIcon, _ := sel.Find("div.Profile-playerSummary--role img").Attr("src")
		if rating, ok := buildRating(sel, roleIcon); ok {
			ps.Ratings = append(ps.Ratings, rating)
		}
	})
}

// parseDetailedStats populates the passed stats collection with detailed statistics
func parseDetailedStats(platform Platform, playMode string, sc *StatsCollection) {
	sc.TopHeroes = parseHeroStats(platform.ProfileView.Find(".Profile-heroSummary--view" + playMode))
	sc.CareerStats = parseCareerStats(platform.ProfileView.Find(".stats" + playMode))

	// Backfill missing TopHeroes data from CareerStats
	for heroName, topStats := range sc.TopHeroes {
		if careerStats, ok := sc.CareerStats[heroName]; ok {

			// Helper closure to parse int from percent string or int value
			parseVal := func(m map[string]any, key string) int {
				if v, ok := m[key]; ok {
					switch val := v.(type) {
					case int:
						return val
					case float64:
						return int(val)
					case string:
						// Handle percentages "49%"
						clean := strings.ReplaceAll(val, "%", "")
						clean = strings.ReplaceAll(clean, ",", "")
						if i, err := strconv.Atoi(clean); err == nil {
							return i
						}
					}
				}
				return 0
			}

			// Helper for float parsing
			parseFloat := func(m map[string]any, key string) float64 {
				if v, ok := m[key]; ok {
					switch val := v.(type) {
					case float64:
						return val
					case int:
						return float64(val)
					case string:
						clean := strings.ReplaceAll(val, "%", "")
						clean = strings.ReplaceAll(clean, ",", "")
						if f, err := strconv.ParseFloat(clean, 64); err == nil {
							return f
						}
					}
				}
				return 0
			}

			// Backfill logic
			if topStats.WeaponAccuracy == 0 {
				topStats.WeaponAccuracy = parseVal(careerStats.Combat, "weaponAccuracy")
			}
			if topStats.CriticalHitAccuracy == 0 {
				topStats.CriticalHitAccuracy = parseVal(careerStats.Combat, "criticalHitAccuracy")
				if topStats.CriticalHitAccuracy == 0 {
					topStats.CriticalHitAccuracy = parseVal(careerStats.HeroSpecific, "criticalHitAccuracy") // Try hero specific
				}
				if topStats.CriticalHitAccuracy == 0 {
					topStats.CriticalHitAccuracy = parseVal(careerStats.HeroSpecific, "scopedAccuracy") // Ana specific example
				}
			}
			if topStats.ObjectiveKills == 0 {
				topStats.ObjectiveKills = parseFloat(careerStats.Combat, "objectiveKills")
			}

			// New fields backfill
			if topStats.GamesPlayed == 0 {
				topStats.GamesPlayed = parseVal(careerStats.Game, "gamesPlayed")
			}
			if topStats.GamesLost == 0 {
				topStats.GamesLost = parseVal(careerStats.Game, "gamesLost")
			}
			if topStats.WinPercentage == 0 {
				topStats.WinPercentage = parseVal(careerStats.Game, "winPercentage")
			}
			if topStats.ObjectiveKillsBest == 0 {
				topStats.ObjectiveKillsBest = parseVal(careerStats.Best, "objectiveKillsMostInGame")
			}
			if topStats.HealingDoneBest == 0 {
				topStats.HealingDoneBest = parseVal(careerStats.Best, "healingDoneMostInGame")
			}
			if topStats.DamageDoneBest == 0 {
				topStats.DamageDoneBest = parseVal(careerStats.Best, "allDamageDoneMostInGame")
			}
			if topStats.KillStreakBest == 0 {
				topStats.KillStreakBest = parseVal(careerStats.Best, "killsStreakBest")
			}
			if topStats.MultiKillBest == 0 {
				topStats.MultiKillBest = parseVal(careerStats.Best, "multikillsBest")
			}
			if topStats.EliminationsPerLife == 0 {
				topStats.EliminationsPerLife = parseFloat(careerStats.Average, "eliminationsPerLife")
			}
		}
	}
}

// parseHeroStats : Parses stats for each individual hero and returns a map
func parseHeroStats(heroStatsSelector *goquery.Selection) map[string]*TopHeroStats {
	bhsMap := make(map[string]*TopHeroStats)
	categoryMap := make(map[string]string)

	heroStatsSelector.Find(".topheroes-dropdown option").Each(func(i int, sel *goquery.Selection) {
		optionName := sel.Text()
		optionVal, _ := sel.Attr("value")

		categoryMap[optionVal] = cleanJSONKey(optionName)
	})

	heroStatsSelector.Find("div.Profile-progressBars").Each(func(i int, heroGroupSel *goquery.Selection) {
		categoryID, _ := heroGroupSel.Attr("data-category-id")
		categoryID = categoryMap[categoryID]

		heroGroupSel.Find(".Profile-progressBar").Each(func(i2 int, statSel *goquery.Selection) {
			heroName := cleanJSONKey(statSel.Find(".Profile-progressBar-title").Text())
			statVal := statSel.Find(".Profile-progressBar-description").Text()

			// Creates hero map if it doesn't exist
			if bhsMap[heroName] == nil {
				bhsMap[heroName] = new(TopHeroStats)
			}

			if bhsMap[heroName].HeroPicture == "" {
				if src, ok := statSel.Find("img.Profile-progressBar--icon").Attr("src"); ok {
					src = strings.TrimSpace(src)
					if src != "" {
						bhsMap[heroName].HeroPicture = src
					}
				}
			}

			// Sets hero stats based on stat category type
			switch categoryID {
			case "timePlayed":
				bhsMap[heroName].TimePlayed = statVal
			case "gamesWon":
				bhsMap[heroName].GamesWon, _ = strconv.Atoi(statVal)
			case "gamesPlayed":
				bhsMap[heroName].GamesPlayed, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "gamesLost":
				bhsMap[heroName].GamesLost, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "winPercentage":
				bhsMap[heroName].WinPercentage, _ = strconv.Atoi(strings.ReplaceAll(statVal, "%", ""))

			case "eliminationsPerLife":
				bhsMap[heroName].EliminationsPerLife, _ = strconv.ParseFloat(statVal, 64)
			case "multikillBest":
				bhsMap[heroName].MultiKillBest, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "objectiveKills":
				bhsMap[heroName].ObjectiveKills, _ = strconv.ParseFloat(statVal, 64)
			case "objectiveKillsBest":
				bhsMap[heroName].ObjectiveKillsBest, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "healingDoneBest":
				bhsMap[heroName].HealingDoneBest, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "damageDoneBest":
				bhsMap[heroName].DamageDoneBest, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			case "killStreakBest":
				bhsMap[heroName].KillStreakBest, _ = strconv.Atoi(strings.ReplaceAll(statVal, ",", ""))
			}
		})
	})
	return bhsMap
}

// parseCareerStats
func parseCareerStats(careerStatsSelector *goquery.Selection) map[string]*CareerStats {
	csMap := make(map[string]*CareerStats)
	heroMap := make(map[string]string)

	// Populates tempHeroMap to match hero ID to name in second scrape
	careerStatsSelector.Find(".blz-dropdown option").Each(func(i int, heroSel *goquery.Selection) {
		heroVal, _ := heroSel.Attr("value")
		heroMap[heroVal] = heroSel.Text()
	})

	// Iterates over every hero div
	careerStatsSelector.Find(".stats-container").Each(func(i int, heroStatsSel *goquery.Selection) {
		classAttributes, _ := heroStatsSel.Attr("class")

		var currentHeroOption string

		for _, class := range strings.Fields(classAttributes) {
			if !strings.HasPrefix(class, "option-") {
				continue
			}

			currentHeroOption = class[strings.Index(class, "-")+1:]
		}

		currentHero, exists := heroMap[currentHeroOption]

		if currentHeroOption == "" || !exists {
			return
		}

		currentHero = cleanJSONKey(currentHero)

		// Iterates over every stat box
		heroStatsSel.Find("div.category").Each(func(i2 int, statBoxSel *goquery.Selection) {
			statType := statBoxSel.Find(".header p").Text()
			statType = cleanJSONKey(statType)

			// Iterates over stat row
			statBoxSel.Find(".stat-item").Each(func(i3 int, statSel *goquery.Selection) {
				statKey := transformKey(cleanJSONKey(statSel.Find(".name").Text()))
				statVal := strings.ReplaceAll(statSel.Find(".value").Text(), ",", "") // Removes commas from 1k+ values
				statVal = strings.TrimSpace(statVal)

				// Creates stat map if it doesn't exist
				if csMap[currentHero] == nil {
					csMap[currentHero] = new(CareerStats)
				}

				// Switches on type, creating category stat maps if exists (will omitempty on json marshal)
				switch statType {
				case "assists":
					if csMap[currentHero].Assists == nil {
						csMap[currentHero].Assists = make(map[string]any)
					}
					csMap[currentHero].Assists[statKey] = parseType(statVal)
				case "average":
					if csMap[currentHero].Average == nil {
						csMap[currentHero].Average = make(map[string]any)
					}
					csMap[currentHero].Average[statKey] = parseType(statVal)
				case "best":
					if csMap[currentHero].Best == nil {
						csMap[currentHero].Best = make(map[string]any)
					}
					csMap[currentHero].Best[statKey] = parseType(statVal)
				case "combat":
					if csMap[currentHero].Combat == nil {
						csMap[currentHero].Combat = make(map[string]any)
					}
					csMap[currentHero].Combat[statKey] = parseType(statVal)
				case "deaths":
					if csMap[currentHero].Deaths == nil {
						csMap[currentHero].Deaths = make(map[string]any)
					}
					csMap[currentHero].Deaths[statKey] = parseType(statVal)
				case "heroSpecific":
					if csMap[currentHero].HeroSpecific == nil {
						csMap[currentHero].HeroSpecific = make(map[string]any)
					}
					csMap[currentHero].HeroSpecific[statKey] = parseType(statVal)
				case "game":
					if csMap[currentHero].Game == nil {
						csMap[currentHero].Game = make(map[string]any)
					}
					csMap[currentHero].Game[statKey] = parseType(statVal)
				case "matchAwards":
					if csMap[currentHero].MatchAwards == nil {
						csMap[currentHero].MatchAwards = make(map[string]any)
					}
					csMap[currentHero].MatchAwards[statKey] = parseType(statVal)
				}
			})
		})
	})
	return csMap
}

func parseType(val string) any {
	i, err := strconv.Atoi(val)
	if err == nil {
		return i
	}
	f, err := strconv.ParseFloat(val, 64)
	if err == nil {
		return f
	}
	return val
}

var (
	keyReplacer = strings.NewReplacer("-", " ", ".", " ", ":", " ", "'", "", "ú", "u", "ö", "o")

	// pluralRegexp strips ICU plural syntax such as
	// "{count, plural, one {Medal} other {Medals}}" down to the "other" form.
	// Compiled once at package level: cleanJSONKey runs hundreds of times per profile.
	pluralRegexp = regexp.MustCompile(`{count, plural, one {.+} other {(.+)}}`)

	// titleCaser replaces the deprecated strings.Title.
	titleCaser = cases.Title(language.English)
)

// cleanJSONKey turns a human readable stat label into a stable lowerCamelCase
// JSON key.
func cleanJSONKey(str string) string {
	// Remove localization boilerplate.
	if strings.Contains(str, "} other {") {
		if otherForm, ok := submatch(pluralRegexp, str, 1); ok {
			str = pluralRegexp.ReplaceAllString(str, otherForm)
		}
	}

	str = keyReplacer.Replace(str) // Removes all dashes, dots, and colons from titles
	str = strings.ToLower(str)
	str = titleCaser.String(str)           // Uppercases lowercase leading characters
	str = strings.ReplaceAll(str, " ", "") // Removes spaces
	for i, v := range str {                // Lowercases the initial character
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}
