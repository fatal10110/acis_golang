package ai

import (
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/bossmemo"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestHeartOfWardingAdmitsWithShippedSpawns boots the lair from the shipped
// spawnlist unchanged, whose Antharas sizes store themselves under GM_ID 1,
// 9 and 10. The Heart reads those keys (owner-approved divergence, #4682:
// the keys it read before belong to Frintezza), so the entry takes a Portal
// Stone, teleports the character into the lair with no page, and the end of
// the boss wait wakes the small Antharas (db value 1) and resets the
// Heart's wait mark and count.
func TestHeartOfWardingAdmitsWithShippedSpawns(t *testing.T) {
	lair, err := os.ReadFile(datapack.Path(t, "data", "xml", "spawnlist", "25_21.xml"))
	if err != nil {
		t.Fatal(err)
	}
	w := bootHeartLair(t, string(lair), nil, 1)
	frames := enterHeart(t, w)
	if got := baiumPageNames(frames); len(got) != 0 {
		t.Fatalf("the entry's pages = %q, want none", got)
	}
	if got, want := w.characterTeleports(t, frames), []string{"S TeleportToLocation obj=player loc=179700,113800,-7709 fast=0"}; !slices.Equal(got, want) {
		t.Fatalf("the entry's teleports = %q, want %q", got, want)
	}
	w.appear(t)
	if got := stonesHeld(t, w); got != 0 {
		t.Fatalf("Portal Stones held = %d, want 0", got)
	}
	if waiting, entrants := heartCounts(t, w); waiting != 1 || entrants != 1 {
		t.Fatalf("the Heart's wait mark and entrants = %d, %d, want 1, 1", waiting, entrants)
	}

	w.advance(t, 30*time.Minute)
	if got, want := heartDBValues(t, w), map[int32]int{29066: 1, 29067: 0, 29068: 0}; !maps.Equal(got, want) {
		t.Fatalf("db values once the wait ends = %v, want %v", got, want)
	}
	if waiting, entrants := heartCounts(t, w); waiting != 0 || entrants != 0 {
		t.Fatalf("the Heart's wait mark and entrants after the wake = %d, %d, want 0, 0", waiting, entrants)
	}
}

// TestHeartOfWardingTreatsANonNPCKeyAsMissing has the small Antharas's key
// name the character instead of an NPC. The Heart answers 13001-02.htm as
// for a missing Antharas, keeping the character's stone and place
// (owner-approved divergence, #4682: the entry no longer aborts).
func TestHeartOfWardingTreatsANonNPCKeyAsMissing(t *testing.T) {
	w := bootHeart(t, nil, 1)
	w.srv.RunScript(t, w.player.ObjectID(), heartDirector, func(s *script.Script, _ *script.Player) {
		s.GlobalMemo().Set(bossmemo.AntharasSmall, int(w.player.ObjectID()))
	})
	frames := enterHeart(t, w)
	if got, want := baiumPageNames(frames), []string{"13001-02.htm"}; !slices.Equal(got, want) {
		t.Fatalf("the entry's pages = %q, want %q", got, want)
	}
	if got := w.characterTeleports(t, frames); len(got) != 0 {
		t.Fatalf("the entry's teleports = %q, want none", got)
	}
	if got := stonesHeld(t, w); got != 1 {
		t.Fatalf("Portal Stones held = %d, want 1", got)
	}
	if waiting, entrants := heartCounts(t, w); waiting != 0 || entrants != 0 {
		t.Fatalf("the Heart's wait mark and entrants = %d, %d, want 0, 0", waiting, entrants)
	}
}
