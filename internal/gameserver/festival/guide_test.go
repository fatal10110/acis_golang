package festival

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Each oracle's five guides, then its five witches, serve the festivals in
// order, the signup fee rising with the level range; no other NPC is a
// guide.
func TestGuideOf(t *testing.T) {
	fees := [Count][3]int{{900, 540, 270}, {1500, 900, 450}, {3000, 1800, 900}, {4500, 2700, 1350}, {6000, 3600, 1800}}
	for _, oracle := range []struct {
		first int
		cabal sevensigns.Cabal
	}{{31127, sevensigns.Dawn}, {31137, sevensigns.Dusk}} {
		for i := range 2 * Count {
			id := i % Count
			want := Guide{FestivalID: id, Cabal: oracle.cabal, Blue: fees[id][0], Green: fees[id][1], Red: fees[id][2]}
			if got, ok := GuideOf(oracle.first + i); !ok || got != want {
				t.Errorf("GuideOf(%d) = %+v, %v; want %+v", oracle.first+i, got, ok, want)
			}
		}
	}
	for _, npcID := range []int{31126, 31147, 0} {
		if g, ok := GuideOf(npcID); ok {
			t.Errorf("GuideOf(%d) = %+v, want no guide", npcID, g)
		}
	}
}

// A score's date reads year/month/day in the server's zone, January being
// month 0.
func TestScoreDate(t *testing.T) {
	d := time.Date(2026, time.January, 5, 12, 0, 0, 0, time.Local)
	if got := scoreDate(d.UnixMilli()); got != "2026/0/5" {
		t.Fatalf("scoreDate = %q, want 2026/0/5", got)
	}
	d = time.Date(2025, time.December, 31, 12, 0, 0, 0, time.Local)
	if got := scoreDate(d.UnixMilli()); got != "2025/11/31" {
		t.Fatalf("scoreDate = %q, want 2025/11/31", got)
	}
}
