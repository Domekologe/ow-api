package seasonmap

import (
	"encoding/json"
	"os"
	"sort"
)

// ResetsFile is the on-disk JSON shape for season reset anchors.
type ResetsFile struct {
	Resets []int `json:"resets"`
}

// ReadResetsFile loads reset anchors from path. Missing file yields nil, nil.
func ReadResetsFile(path string) ([]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f ResetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return NormalizeResets(f.Resets), nil
}

// NormalizeResets returns a sorted, de-duplicated copy with non-positive values dropped.
func NormalizeResets(in []int) []int {
	seen := make(map[int]struct{})
	for _, v := range in {
		if v > 0 {
			seen[v] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// RealSeason maps a scraper/API season number to the displayed season after applying
// reset anchors. Each anchor is the first season number of a new cycle (displayed as 1).
// Anchors must be sorted ascending (NormalizeResets).
func RealSeason(scraped int, anchors []int) int {
	var anchor int
	found := false
	for _, r := range anchors {
		if r <= scraped && (!found || r > anchor) {
			anchor = r
			found = true
		}
	}
	if !found {
		return scraped
	}
	return scraped - anchor + 1
}
