package seasonmap

import "github.com/Domekologe/ow-api/ovrstat"

// ApplyPlayerStats sets competitiveStats.realSeason for every non-nil season.
func ApplyPlayerStats(ps *ovrstat.PlayerStats, anchors []int) {
	if ps == nil {
		return
	}
	applyCompetitiveCollection(&ps.CompetitiveStats, anchors)
}

// ApplyPlayerProfile sets competitiveStats.realSeason for every non-nil season.
func ApplyPlayerProfile(ps *ovrstat.PlayerStatsProfile, anchors []int) {
	if ps == nil {
		return
	}
	applySummary(&ps.CompetitiveStats, anchors)
}

func applyCompetitiveCollection(coll *ovrstat.CompetitiveStatsCollection, anchors []int) {
	if coll == nil {
		return
	}
	if coll.Season == nil {
		coll.RealSeason = nil
		return
	}
	real := RealSeason(*coll.Season, anchors)
	coll.RealSeason = intPtr(real)
}

func applySummary(sum *ovrstat.CompetitiveSummary, anchors []int) {
	if sum == nil {
		return
	}
	if sum.Season == nil {
		sum.RealSeason = nil
		return
	}
	real := RealSeason(*sum.Season, anchors)
	sum.RealSeason = intPtr(real)
}

func intPtr(v int) *int {
	return &v
}
