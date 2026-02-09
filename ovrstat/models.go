package ovrstat

import "github.com/PuerkitoBio/goquery"

// PlayerStats holds all stats on a specified Overwatch player
type PlayerStats struct {
	Icon             string                     `json:"icon"`
	Name             string                     `json:"name"`
	Endorsement      int                        `json:"endorsement"`
	EndorsementIcon  string                     `json:"endorsementIcon"`
	Title            string                     `json:"title"`
	NamecardID       string                     `json:"namecardId,omitempty"`
	NamecardTitle    string                     `json:"namecardTitle,omitempty"`
	NamecardImage    string                     `json:"namecardImage"`
	Ratings          []Rating                   `json:"ratings"`
	GamesPlayed      int                        `json:"gamesPlayed"`
	GamesWon         int                        `json:"gamesWon"`
	GamesLost        int                        `json:"gamesLost"`
	QuickPlayStats   QuickPlayStatsCollection   `json:"quickPlayStats"`
	CompetitiveStats CompetitiveStatsCollection `json:"competitiveStats"`
	Private          bool                       `json:"private"`
}

type PlayerStatsProfile struct {
	Icon             string             `json:"icon"`
	Name             string             `json:"name"`
	Endorsement      int                `json:"endorsement"`
	EndorsementIcon  string             `json:"endorsementIcon"`
	Title            string             `json:"title"`
	NamecardID       string             `json:"namecardId,omitempty"`
	NamecardTitle    string             `json:"namecardTitle,omitempty"`
	NamecardImage    string             `json:"namecardImage"`
	CompetitiveStats CompetitiveSummary `json:"competitiveStats,omitempty"`
	QuickplayStats   QuickplaySummary   `json:"quickplayStats,omitempty"`
	Ratings          []Rating           `json:"ratings"`
	Private          bool               `json:"private"`
}

type Rating struct {
	Group    string `json:"group"`
	Tier     int    `json:"tier"`
	Role     string `json:"role"`
	RoleIcon string `json:"roleIcon"`
	RankIcon string `json:"rankIcon"`
	TierIcon string `json:"tierIcon"`
}

type CompetitiveSummary struct {
	Season                      *int   `json:"season,omitempty"`
	GamesPlayed                 int    `json:"gamesPlayed,omitempty"`
	GamesWon                    int    `json:"gamesWon,omitempty"`
	GamesLost                   int    `json:"gamesLost,omitempty"`
	TimePlayed                  string `json:"timePlayed,omitempty"`
	MostPlayedHero              string `json:"mostPlayedHero,omitempty"`
	MostPlayedHeroTimePlayed    string `json:"mostPlayedHeroTimePlayed,omitempty"`
	MostPlayedHeroGamesPlayed   int    `json:"mostPlayedHeroGamesPlayed,omitempty"`
	MostPlayedHeroWinPercentage int    `json:"mostPlayedHeroWinPercentage,omitempty"`
}

type QuickplaySummary struct {
	GamesPlayed                 int    `json:"gamesPlayed,omitempty"`
	GamesWon                    int    `json:"gamesWon,omitempty"`
	GamesLost                   int    `json:"gamesLost,omitempty"`
	TimePlayed                  string `json:"timePlayed,omitempty"`
	MostPlayedHero              string `json:"mostPlayedHero,omitempty"`
	MostPlayedHeroTimePlayed    string `json:"mostPlayedHeroTimePlayed,omitempty"`
	MostPlayedHeroGamesPlayed   int    `json:"mostPlayedHeroGamesPlayed,omitempty"`
	MostPlayedHeroWinPercentage int    `json:"mostPlayedHeroWinPercentage,omitempty"`
}

type StatsCollection struct {
	TopHeroes   map[string]*TopHeroStats `json:"topHeroes"`
	CareerStats map[string]*CareerStats  `json:"careerStats"`
}

type CompetitiveStatsCollection struct {
	Season *int `json:"season"`
	StatsCollection
}

type QuickPlayStatsCollection struct {
	StatsCollection
}

// TopHeroStats holds basic stats for each hero
type TopHeroStats struct {
	TimePlayed          string  `json:"timePlayed"`
	GamesWon            int     `json:"gamesWon"`
	WeaponAccuracy      int     `json:"weaponAccuracy"`
	CriticalHitAccuracy int     `json:"criticalHitAccuracy"`
	EliminationsPerLife float64 `json:"eliminationsPerLife"`
	MultiKillBest       int     `json:"multiKillBest"`
	ObjectiveKills      float64 `json:"objectiveKills"`
	Valid               bool    `json:"-"` // Used to determine if the stats are valid
	GamesPlayed         int     `json:"gamesPlayed"`
	GamesLost           int     `json:"gamesLost"`
	WinPercentage       int     `json:"winPercentage"`
	ObjectiveKillsBest  int     `json:"objectiveKillsBest"`
	HealingDoneBest     int     `json:"healingDoneBest"`
	DamageDoneBest      int     `json:"damageDoneBest"`
	KillStreakBest      int     `json:"killStreakBest"`
}

// CareerStats holds very detailed stats for each hero
type CareerStats struct {
	Assists      map[string]interface{} `json:"assists"`
	Average      map[string]interface{} `json:"average"`
	Best         map[string]interface{} `json:"best"`
	Combat       map[string]interface{} `json:"combat"`
	HeroSpecific map[string]interface{} `json:"heroSpecific"`
	Game         map[string]interface{} `json:"game"`
	MatchAwards  map[string]interface{} `json:"matchAwards"`

	// Deaths appears to have been removed, so we hide it.
	Deaths map[string]interface{} `json:"deaths,omitempty"`
}

// Player represents a response from the search-by-name api request
type Player struct {
	BattleTag string `json:"battleTag"`
	Portrait  string `json:"portrait"`
	Frame     string `json:"frame"`
	IsPublic  bool   `json:"isPublic"`
	Namecard  string `json:"namecard"`
}

// Platform represents a supported platform (PC, Console)
type Platform struct {
	Name        string
	Active      bool
	RankWrapper *goquery.Selection
	ProfileView *goquery.Selection
}

type UnlockData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}
