package seasonmap

import "testing"

func TestRealSeason(t *testing.T) {
	anchors := []int{21, 41}
	tests := []struct {
		scraped int
		want    int
	}{
		{19, 19},
		{20, 20},
		{21, 1},
		{22, 2},
		{40, 20},
		{41, 1},
		{42, 2},
		{50, 10},
	}
	for _, tt := range tests {
		if got := RealSeason(tt.scraped, anchors); got != tt.want {
			t.Errorf("RealSeason(%d, %v) = %d; want %d", tt.scraped, anchors, got, tt.want)
		}
	}
}

func TestRealSeason_noAnchors(t *testing.T) {
	if got := RealSeason(99, nil); got != 99 {
		t.Errorf("got %d, want 99", got)
	}
}

func TestNormalizeResets(t *testing.T) {
	got := NormalizeResets([]int{41, 21, 21, -1, 0})
	want := []int{21, 41}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}
