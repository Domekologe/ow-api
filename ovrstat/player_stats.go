package ovrstat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
)

const (
	baseURL = "https://overwatch.blizzard.com/en-us/career"

	// API URL Use '#' as A Delimiter between Number and Name and not '-'
	apiURL = "https://overwatch.blizzard.com/en-us/search/account-by-name/"

	// PlatformPC is a platform for PCs (mouseKeyboard in the page)
	PlatformPC = "pc"

	// PlatformConsole is a consolidated platform of all consoles
	PlatformConsole = "console"
)

var (
	// ErrPlayerNotFound is thrown when a player doesn't exist
	ErrPlayerNotFound = errors.New("Player not found! No Players Found!")

	// ErrInvalidPlatform is thrown when the passed params are incorrect
	ErrInvalidPlatform = errors.New("Invalid platform")

	owClient *http.Client

	Debug = true
)

func getOWClient() *http.Client {
	if owClient != nil {
		return owClient
	}
	jar, _ := cookiejar.New(nil)
	owClient = &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
	if Debug {
		fmt.Println("[DEBUG] http.Client initialisiert mit CookieJar")
	}
	return owClient
}

func primeOWSession(c *http.Client) error {
	url := "https://overwatch.blizzard.com/en-us/search/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("prime build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ovrstat)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	if Debug {
		fmt.Println("[DEBUG] Prime Request URL:", url)
		fmt.Println("[DEBUG] Prime Request Headers:", req.Header)
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("prime do: %w", err)
	}
	defer resp.Body.Close()

	if Debug {
		fmt.Printf("[DEBUG] Prime Response Status: %d %s\n", resp.StatusCode, resp.Status)
	}

	io.Copy(io.Discard, resp.Body)

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

func resolvePlayerByDoubleSearch(tag string) (*Player, error) {
	// 1️⃣ Existenz über Career-Redirect prüfen
	_, err := resolveCareerID(tag)
	if err != nil {
		return nil, ErrPlayerNotFound
	}

	// 2️⃣ Name extrahieren
	name, full := splitTag(tag)

	// 3️⃣ Search nur noch als Metadatenquelle
	playersByName, _ := retrievePlayers(name)
	playersByFull, _ := retrievePlayers(full)

	// 4️⃣ Wenn Full-Search was liefert → nehmen
	if len(playersByFull) > 0 {
		return &playersByFull[0], nil
	}

	// 5️⃣ Fallback: Name-Search (einziger Treffer)
	if len(playersByName) == 1 {
		return &playersByName[0], nil
	}

	// 6️⃣ Existiert zwar, aber nicht eindeutig auffindbar
	return &Player{
		BattleTag: strings.ReplaceAll(tag, "-", "#"),
		IsPublic:  true, // unknown → default
	}, nil
}

// Adding function to convert ID from Search to usable URL
func GetUnlockInfo(unlockID string) (*UnlockData, error) {
	c := getOWClient()

	// Session Prepare
	if err := primeOWSession(c); err != nil {
		return nil, err
	}

	base := "https://overwatch.blizzard.com/en-us/search/unlocks/"
	q := url.Values{}
	q.Set("unlockIds", unlockID)
	fullURL := base + "?" + q.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://overwatch.blizzard.com/en-us/search/")
	req.Header.Set("Accept-Language", "en,de-DE;q=0.9,de;q=0.8,en-US;q=0.7")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-CH-UA", "\"Chromium\";v=\"135\", \"Not-A.Brand\";v=\"99\"")
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", "\"Windows\"")

	if Debug {
		fmt.Println("[DEBUG] Unlock Request URL:", fullURL)
		fmt.Println("[DEBUG] Unlock Request Headers:", req.Header)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if Debug {
		fmt.Printf("[DEBUG] Unlock Response Status: %d %s\n", resp.StatusCode, resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		if Debug {
			fmt.Println("[DEBUG] Unlock Response Body:", string(b))
		}
		return nil, fmt.Errorf("unlocks %d: %s", resp.StatusCode, string(b))
	}

	var unlocks []UnlockData
	if err := json.NewDecoder(resp.Body).Decode(&unlocks); err != nil {
		return nil, err
	}

	if Debug {
		fmt.Printf("[DEBUG] Decoded Unlocks (%d): %+v\n", len(unlocks), unlocks)
	}

	for _, u := range unlocks {
		if u.ID == unlockID {
			if Debug {
				fmt.Println("[DEBUG] Match found:", u)
			}
			return &u, nil
		}
	}

	if Debug {
		fmt.Println("[DEBUG] Unlock not found for ID:", unlockID)
	}
	return nil, fmt.Errorf("Unlock ID %s not found", unlockID)
}

func resolveCareerID(tag string) (string, error) {
	tag = strings.ReplaceAll(tag, "#", "-")

	if Debug {
		fmt.Println("[DEBUG] resolveCareerID input tag:", tag)
		fmt.Println("[DEBUG] resolveCareerID URL:", baseURL+"/"+tag)
	}

	jar, _ := cookiejar.New(nil)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest("GET", baseURL+"/"+tag, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ovrstat)")
	req.Header.Set("Accept", "text/html")

	for i := 0; i < 5; i++ {
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		if Debug {
			fmt.Println("[DEBUG] Redirect step", i)
			fmt.Println("[DEBUG] Status:", resp.StatusCode)
			fmt.Println("[DEBUG] Location:", resp.Header.Get("Location"))
			fmt.Println("[DEBUG] Final URL:", resp.Request.URL.String())
		}

		// 🔥 FALL 1: Redirect mit Location
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				resp.Body.Close()
				return "", errors.New("redirect without location")
			}

			decodedLoc, _ := url.PathUnescape(loc)

			if strings.Contains(decodedLoc, "|") {
				id := strings.Trim(strings.TrimPrefix(decodedLoc, "/career/"), "/")
				resp.Body.Close()
				return id, nil
			}

			nextURL := loc
			if strings.HasPrefix(loc, "/") {
				nextURL = "https://overwatch.blizzard.com" + loc
			}

			resp.Body.Close()
			req, _ = http.NewRequest("GET", nextURL, nil)
			continue
		}

		// 🔥 FALL 2: Finaler 200-Request → URL auswerten
		if resp.StatusCode == 200 {
			finalPath := resp.Request.URL.Path
			decodedPath, _ := url.PathUnescape(finalPath)

			if strings.Contains(decodedPath, "|") {
				id := strings.Trim(strings.TrimPrefix(decodedPath, "/en-us/career/"), "/")
				resp.Body.Close()
				return id, nil
			}
		}

		resp.Body.Close()
		break
	}

	if Debug {
		fmt.Println("[DEBUG] resolveCareerID FAILED for tag:", tag)
	}

	return "", ErrPlayerNotFound
}

// Stats retrieves player stats
// Universal method if you don't need to differentiate it
func Stats(platformKey, tag string) (*PlayerStats, error) {
	// Do platform key mapping
	switch platformKey {
	case PlatformPC:
		platformKey = "mouseKeyboard"
	case PlatformConsole:
		platformKey = "controller"
	}

	// Parse the API response first
	var ps PlayerStats

	player, err := resolvePlayerByDoubleSearch(tag)
	if err != nil {
		return nil, err
	}

	if !player.IsPublic {
		ps.Private = true
		return &ps, nil
	}

	// Optional (wenn du es brauchst)
	ps.NamecardImage = player.Namecard

	// Create the profile url for scraping
	// Change Minus in Name with '#' for correct searches
	careerID, err := resolveCareerID(tag)
	if err != nil {
		return nil, err
	}

	profileUrl := baseURL + "/" + careerID + "/"

	if Debug {
		fmt.Println("[DEBUG] Resolved CareerID:", careerID)
		fmt.Println("[DEBUG] Profile URL:", profileUrl)
	}

	// Perform the stats request and decode the response
	res, err := getOWClient().Get(profileUrl)

	if err != nil {
		return nil, errors.Wrap(err, "Failed to retrieve profile")
	}
	defer res.Body.Close()

	// Parses the stats request into a goquery document
	pd, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create goquery document")
	}

	// Checks if profile not found, site still returns 200 in this case
	if pd.Find("[slot=heading]").First().Text() == "Page Not Found" {
		return nil, ErrPlayerNotFound
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	platforms := make(map[string]Platform)

	pd.Find(".Profile-player--filters .Profile-player--filter").Each(func(i int, sel *goquery.Selection) {
		id, _ := sel.Attr("id")

		id = filterRegexp.FindStringSubmatch(id)[1]

		viewID := "." + id + "-view"

		// Using combined classes (.class.class2) we can filter out our views based on platform
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

	platform, exists := platforms[platformKey]

	if !exists {
		return nil, ErrInvalidPlatform
	}

	// Scrapes all stats for the passed user and sets struct member data
	parseGeneralInfo(platform, pd.Find(".Profile-masthead").First(), &ps)

	parseDetailedStats(platform, ".quickPlay-view", &ps.QuickPlayStats.StatsCollection)
	parseDetailedStats(platform, ".competitive-view", &ps.CompetitiveStats.StatsCollection)

	competitiveSeason, _ := pd.Find("[data-latestherostatrankseasonow2]").Attr("data-latestherostatrankseasonow2")

	if competitiveSeason != "" {
		competitiveSeason, _ := strconv.Atoi(competitiveSeason)

		ps.CompetitiveStats.Season = &competitiveSeason
	}

	addGameStats(&ps, &ps.QuickPlayStats.StatsCollection)
	addGameStats(&ps, &ps.CompetitiveStats.StatsCollection)

	return &ps, nil
}

func addGameStats(ps *PlayerStats, statsCollection *StatsCollection) {
	if heroStats, ok := statsCollection.CareerStats["allHeroes"]; ok {
		if gamesPlayed, ok := heroStats.Game["gamesPlayed"]; ok {
			ps.GamesPlayed += gamesPlayed.(int)
		}

		if gamesWon, ok := heroStats.Game["gamesWon"]; ok {
			ps.GamesWon += gamesWon.(int)
		}

		if gamesLost, ok := heroStats.Game["gamesLost"]; ok {
			ps.GamesLost += gamesLost.(int)
		}
	}
}

// Only Gets Profile Stats
// This part is and will be mainly used in OWidget 2 Application

func ProfileStats(platformKey, tag string) (*PlayerStatsProfile, error) {
	// Do platform key mapping
	switch platformKey {
	case PlatformPC:
		platformKey = "mouseKeyboard"

	case PlatformConsole:
		platformKey = "controller"
	}
	// Parse the API response first
	var ps PlayerStatsProfile

	player, err := resolvePlayerByDoubleSearch(tag)
	if err != nil {
		return nil, err
	}

	if !player.IsPublic {
		ps.Private = true
		return &ps, nil
	}

	// Optional (wenn du es brauchst)
	ps.NamecardImage = player.Namecard

	// Create the profile url for scraping
	careerID, err := resolveCareerID(tag)
	if err != nil {
		return nil, err
	}

	profileUrl := baseURL + "/" + careerID + "/"

	if Debug {
		fmt.Println("[DEBUG] Resolved CareerID:", careerID)
		fmt.Println("[DEBUG] Profile URL:", profileUrl)
	}

	// Perform the stats request and decode the response
	res, err := getOWClient().Get(profileUrl)

	if err != nil {
		return nil, errors.Wrap(err, "Failed to retrieve profile")
	}
	defer res.Body.Close()

	// Parses the stats request into a goquery document
	pd, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create goquery document")
	}

	// Checks if profile not found, site still returns 200 in this case
	if pd.Find("[slot=heading]").First().Text() == "Page Not Found" {
		return nil, ErrPlayerNotFound
	}

	ps.Name = pd.Find(".Profile-player--name").Text()

	platforms := make(map[string]Platform)

	pd.Find(".Profile-player--filters .Profile-player--filter").Each(func(i int, sel *goquery.Selection) {
		id, _ := sel.Attr("id")

		id = filterRegexp.FindStringSubmatch(id)[1]

		viewID := "." + id + "-view"

		// Using combined classes (.class.class2) we can filter out our views based on platform
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

	platform, exists := platforms[platformKey]

	if !exists {
		return nil, ErrInvalidPlatform
	}

	// Scrapes all stats for the passed user and sets struct member data
	parseGeneralInfoProfile(platform, pd.Find(".Profile-masthead").First(), &ps)

	careerStats := parseCareerStats(platform.ProfileView.Find(".stats.competitive-view"))

	if heroStats, ok := careerStats["allHeroes"]; ok {
		if gamesPlayed, ok := heroStats.Game["gamesPlayed"]; ok {
			ps.CompetitiveStats.GamesPlayed = gamesPlayed.(int)
		}
		if gamesWon, ok := heroStats.Game["gamesWon"]; ok {
			ps.CompetitiveStats.GamesWon = gamesWon.(int)
		}
		if gamesLost, ok := heroStats.Game["gamesLost"]; ok {
			ps.CompetitiveStats.GamesLost = gamesLost.(int)
		}
		if timePlayed, ok := heroStats.Game["timePlayed"]; ok {
			ps.CompetitiveStats.TimePlayed = timePlayed.(string)
		}
	}
	careerStatsQP := parseCareerStats(platform.ProfileView.Find(".stats.quickPlay-view"))
	if seasonAttr, exists := pd.Find("[data-latestherostatrankseasonow2]").Attr("data-latestherostatrankseasonow2"); exists {
		if seasonNumber, err := strconv.Atoi(seasonAttr); err == nil {
			ps.CompetitiveStats.Season = &seasonNumber
		}
	}

	if heroStats, ok := careerStatsQP["allHeroes"]; ok {
		if gamesPlayed, ok := heroStats.Game["gamesPlayed"]; ok {
			ps.QuickplayStats.GamesPlayed = gamesPlayed.(int)
		}
		if gamesWon, ok := heroStats.Game["gamesWon"]; ok {
			ps.QuickplayStats.GamesWon = gamesWon.(int)
		}
		if gamesLost, ok := heroStats.Game["gamesLost"]; ok {
			ps.QuickplayStats.GamesLost = gamesLost.(int)
		}
		if timePlayed, ok := heroStats.Game["timePlayed"]; ok {
			ps.QuickplayStats.TimePlayed = timePlayed.(string)
		}
	}

	mostPlayedHero := platform.ProfileView.
		Find(".Profile-heroSummary--view.competitive-view").
		Find(".Profile-progressBar-title").
		First().
		Text()

	ps.CompetitiveStats.MostPlayedHero = strings.TrimSpace(mostPlayedHero)

	mostPlayedHeroQP := platform.ProfileView.
		Find(".Profile-heroSummary--view.quickPlay-view").
		Find(".Profile-progressBar-title").
		First().
		Text()

	ps.QuickplayStats.MostPlayedHero = strings.TrimSpace(mostPlayedHeroQP)

	return &ps, nil
}

func retrievePlayers(tag string) ([]Player, error) {
	if strings.Contains(tag, "-") {
		tag = strings.Replace(tag, "-", "#", -1)
	}
	// Perform api request
	var platforms []Player

	apires, err := http.Get(apiURL + tag)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to perform platform API request")
	}

	defer apires.Body.Close()

	// Decode received JSON
	if err := json.NewDecoder(apires.Body).Decode(&platforms); err != nil {
		return nil, errors.Wrap(err, "Failed to decode platform API response")
	}
	return platforms, nil
}

var (
	endorsementRegexp = regexp.MustCompile("/(\\d+)-([a-z0-9]+)\\.svg")
	rankRegexp        = regexp.MustCompile(`https://\S+Rank_([a-zA-Z]+)Tier-([a-f0-9]+)\.png`)
	tierRegexp        = regexp.MustCompile(`https://\S+TierDivision_(\d+)-[a-f0-9]+\.png`)
	filterRegexp      = regexp.MustCompile("^([a-zA-Z]+)Filter$")
)

// populateGeneralInfo extracts the users general info and returns it in a
// PlayerStats struct

func parseGeneralInfo(platform Platform, s *goquery.Selection, ps *PlayerStats) {
	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement, _ = strconv.Atoi(endorsementRegexp.FindStringSubmatch(ps.EndorsementIcon)[1])
	ps.Title = s.Find(".Profile-player--title").Text()

	// Try to get Namecard
	if namecardAttr, exists := s.Attr("namecard-id"); exists {
		ps.NamecardID = namecardAttr

		// Hole die Info aus Blizzard Unlocks API
		unlockInfo, err := GetUnlockInfo(namecardAttr)
		if err == nil {
			ps.NamecardTitle = unlockInfo.Name
			ps.NamecardImage = unlockInfo.Icon
		}
	}

	// Parse Endorsement Icon path (/svg?path=)
	if strings.Index(ps.EndorsementIcon, "/svg") == 0 {
		q, err := url.ParseQuery(ps.EndorsementIcon[strings.Index(ps.EndorsementIcon, "?")+1:])

		if err == nil && q.Get("path") != "" {
			ps.EndorsementIcon = q.Get("path")
		}
	}

	// Ratings
	// Note that .is-active is the default platform
	platform.RankWrapper.Find(".Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		// Rank selections.

		var roleIconElement = sel.Find(".Profile-playerSummary--role").Nodes[0]

		var roleIcon string
		if roleIconElement.Data == "div" {
			roleIconElement = sel.Find(".Profile-playerSummary--role img").Nodes[0]

			for _, attr := range roleIconElement.Attr {
				if attr.Key == "src" {
					roleIcon = attr.Val
					break
				}
			}
		} else if roleIconElement.Namespace == "svg" {
			roleIconElement = sel.Find(".Profile-playerSummary--role use").Nodes[0]
			for _, attr := range roleIconElement.Attr {
				if attr.Key == "href" {
					roleIcon = attr.Val
					break
				}
			}
		}

		// Format is /(offense|support|tank)-HEX.svg
		role := path.Base(roleIcon)
		role = role[0:strings.Index(role, "-")]
		rankIcon, _ := sel.Find("img.Profile-playerSummary--rank").Attr("src")
		tierIcon, _ := sel.Find("img.Profile-playerSummary--rank").Eq(1).Attr("src")
		rankInfo := rankRegexp.FindStringSubmatch(rankIcon)
		tierInfo := tierRegexp.FindStringSubmatch(tierIcon)
		tier, _ := strconv.Atoi(tierInfo[1])

		ps.Ratings = append(ps.Ratings, Rating{
			Group:    rankInfo[1],
			Tier:     tier,
			Role:     role,
			RoleIcon: roleIcon,
			RankIcon: rankIcon,
			TierIcon: tierIcon,
		})
	})
}

func parseGeneralInfoProfile(platform Platform, s *goquery.Selection, ps *PlayerStatsProfile) {
	// Populates all general player information
	ps.Icon, _ = s.Find(".Profile-player--portrait").Attr("src")
	ps.EndorsementIcon, _ = s.Find(".Profile-playerSummary--endorsement").Attr("src")
	ps.Endorsement, _ = strconv.Atoi(endorsementRegexp.FindStringSubmatch(ps.EndorsementIcon)[1])
	ps.Title = s.Find(".Profile-player--title").Text()

	// Try to get Namecard
	if namecardAttr, exists := s.Attr("namecard-id"); exists {
		ps.NamecardID = namecardAttr

		// Hole die Info aus Blizzard Unlocks API
		unlockInfo, err := GetUnlockInfo(namecardAttr)
		if err == nil {
			ps.NamecardTitle = unlockInfo.Name
			ps.NamecardImage = unlockInfo.Icon
		}
	}

	// Parse Endorsement Icon path (/svg?path=)
	if strings.Index(ps.EndorsementIcon, "/svg") == 0 {
		q, err := url.ParseQuery(ps.EndorsementIcon[strings.Index(ps.EndorsementIcon, "?")+1:])

		if err == nil && q.Get("path") != "" {
			ps.EndorsementIcon = q.Get("path")
		}
	}

	// Ratings
	// Note that .is-active is the default platform
	platform.RankWrapper.Find("div.Profile-playerSummary--roleWrapper").Each(func(i int, sel *goquery.Selection) {
		// Rank selections.

		roleIcon, _ := sel.Find("div.Profile-playerSummary--role img").Attr("src")

		// Format is /(offense|support|...)-HEX.svg
		role := path.Base(roleIcon)
		role = role[0:strings.Index(role, "-")]
		rankIcon, _ := sel.Find("img.Profile-playerSummary--rank").Attr("src")
		tierIcon, _ := sel.Find("img.Profile-playerSummary--rank").Eq(1).Attr("src")
		rankInfo := rankRegexp.FindStringSubmatch(rankIcon)
		tierInfo := tierRegexp.FindStringSubmatch(tierIcon)
		tier, _ := strconv.Atoi(tierInfo[1])

		ps.Ratings = append(ps.Ratings, Rating{
			Group:    rankInfo[1],
			Tier:     tier,
			Role:     role,
			RoleIcon: roleIcon,
			RankIcon: rankIcon,
			TierIcon: tierIcon,
		})
	})
}

// parseDetailedStats populates the passed stats collection with detailed statistics
func parseDetailedStats(platform Platform, playMode string, sc *StatsCollection) {
	sc.TopHeroes = parseHeroStats(platform.ProfileView.Find(".Profile-heroSummary--view" + playMode))
	sc.CareerStats = parseCareerStats(platform.ProfileView.Find(".stats" + playMode))
}

// parseHeroStats : Parses stats for each individual hero and returns a map
func parseHeroStats(heroStatsSelector *goquery.Selection) map[string]*TopHeroStats {
	bhsMap := make(map[string]*TopHeroStats)
	categoryMap := make(map[string]string)

	heroStatsSelector.Find(".Profile-dropdown option").Each(func(i int, sel *goquery.Selection) {
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

			// Sets hero stats based on stat category type
			switch categoryID {
			case "timePlayed":
				bhsMap[heroName].TimePlayed = statVal
			case "gamesWon":
				bhsMap[heroName].GamesWon, _ = strconv.Atoi(statVal)
			case "weaponAccuracy":
				bhsMap[heroName].WeaponAccuracy, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "criticalHitAccuracy":
				bhsMap[heroName].CriticalHitAccuracy, _ = strconv.Atoi(strings.Replace(statVal, "%", "", -1))
			case "eliminationsPerLife":
				bhsMap[heroName].EliminationsPerLife, _ = strconv.ParseFloat(statVal, 64)
			case "multikillBest":
				bhsMap[heroName].MultiKillBest, _ = strconv.Atoi(statVal)
			case "objectiveKills":
				bhsMap[heroName].ObjectiveKills, _ = strconv.ParseFloat(statVal, 64)
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
	careerStatsSelector.Find(".Profile-dropdown option").Each(func(i int, heroSel *goquery.Selection) {
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
				statVal := strings.Replace(statSel.Find(".value").Text(), ",", "", -1) // Removes commas from 1k+ values
				statVal = strings.TrimSpace(statVal)

				// Creates stat map if it doesn't exist
				if csMap[currentHero] == nil {
					csMap[currentHero] = new(CareerStats)
				}

				// Switches on type, creating category stat maps if exists (will omitempty on json marshal)
				switch statType {
				case "assists":
					if csMap[currentHero].Assists == nil {
						csMap[currentHero].Assists = make(map[string]interface{})
					}
					csMap[currentHero].Assists[statKey] = parseType(statVal)
				case "average":
					if csMap[currentHero].Average == nil {
						csMap[currentHero].Average = make(map[string]interface{})
					}
					csMap[currentHero].Average[statKey] = parseType(statVal)
				case "best":
					if csMap[currentHero].Best == nil {
						csMap[currentHero].Best = make(map[string]interface{})
					}
					csMap[currentHero].Best[statKey] = parseType(statVal)
				case "combat":
					if csMap[currentHero].Combat == nil {
						csMap[currentHero].Combat = make(map[string]interface{})
					}
					csMap[currentHero].Combat[statKey] = parseType(statVal)
				case "deaths":
					if csMap[currentHero].Deaths == nil {
						csMap[currentHero].Deaths = make(map[string]interface{})
					}
					csMap[currentHero].Deaths[statKey] = parseType(statVal)
				case "heroSpecific":
					if csMap[currentHero].HeroSpecific == nil {
						csMap[currentHero].HeroSpecific = make(map[string]interface{})
					}
					csMap[currentHero].HeroSpecific[statKey] = parseType(statVal)
				case "game":
					if csMap[currentHero].Game == nil {
						csMap[currentHero].Game = make(map[string]interface{})
					}
					csMap[currentHero].Game[statKey] = parseType(statVal)
				case "matchAwards":
					if csMap[currentHero].MatchAwards == nil {
						csMap[currentHero].MatchAwards = make(map[string]interface{})
					}
					csMap[currentHero].MatchAwards[statKey] = parseType(statVal)
				}
			})
		})
	})
	return csMap
}

func parseType(val string) interface{} {
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
)

// cleanJSONKey
func cleanJSONKey(str string) string {
	// Removes localization rubish
	if strings.Contains(str, "} other {") {
		re := regexp.MustCompile("{count, plural, one {.+} other {(.+)}}")
		if len(re.FindStringSubmatch(str)) == 2 {
			otherForm := re.FindStringSubmatch(str)[1]
			str = re.ReplaceAllString(str, otherForm)
		}
	}

	str = keyReplacer.Replace(str) // Removes all dashes, dots, and colons from titles
	str = strings.ToLower(str)
	str = strings.Title(str)                // Uppercases lowercase leading characters
	str = strings.Replace(str, " ", "", -1) // Removes Spaces
	for i, v := range str {                 // Lowercases initial character
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}
