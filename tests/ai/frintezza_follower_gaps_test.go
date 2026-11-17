package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestFollowerOfFrintezzaJumpsToAnAttackerItCannotReach ports the
// stuck-path teleport of the attacked hook: hit after more than 10 failed
// paths, hurt, by the creature it wants most, the follower is teleported
// onto it; at 10 failed paths it stays.
func TestFollowerOfFrintezzaJumpsToAnAttackerItCannotReach(t *testing.T) {
	for _, row := range []struct {
		name  string
		fails int
		jumps bool
	}{
		{"11 failed paths", 11, true},
		{"10 failed paths", 10, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			w := bootFollower(t, frLiveRow(5), "GrandBoss")
			f := w.follower(t)
			w.hitTo(t, f, 0.9)
			w.onQueue(t, f, func() { script.NPCOf(f).AddAttackDesire(script.PlayerOf(w.player), 1000) })
			if top := script.NPCOf(f).TopDesireTarget(); !script.Same(top, script.PlayerOf(w.player)) {
				t.Fatalf("follower top desire target = %v, want the character", top)
			}
			w.teleport(t, f, 600, 0)
			w.onQueue(t, f, func() {
				for range row.fails {
					f.AddGeoPathFailCount()
				}
			})
			before := frAt(f)

			w.hit(t, f, 1)
			w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := w.player.Position()
			after := frAt(f)
			if row.jumps {
				if after[0] != x || after[1] != y {
					t.Fatalf("follower at %v after the hit, want on the character at %d,%d", after, x, y)
				}
				return
			}
			if after[0] != before[0] || after[1] != before[1] {
				t.Fatalf("follower at %v after the hit, want it kept at %v", after, before)
			}
		})
	}
}
