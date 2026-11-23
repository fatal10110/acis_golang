package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSiegeFlagAgainstSiegeEnd pins #5143: an attacking clan's leader casts
// Build Headquarters while the siege runs, castle or clan hall, and the
// siege ends before the flag lands, after it, or as it lands. Whichever
// comes first, the clan holds no headquarters afterwards and none stands in
// the world.
func TestSiegeFlagAgainstSiegeEnd(t *testing.T) {
	t.Parallel()
	for _, hall := range []bool{false, true} {
		for _, order := range []string{"end first", "flag first", "together"} {
			name := "castle " + order
			if hall {
				name = "hall " + order
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var w *hqWorld
				var end func()
				if hall {
					w = bootHallHQ(t, true)
					s, ok := w.srv.Halls.Siege(devastatedCastleID)
					if !ok {
						t.Fatal("no Devastated Castle siege")
					}
					end = func() { s.End() }
				} else {
					w = bootHQ(t, hqSetup{})
					end = w.gludio.End
				}
				w.c.Send(encodeRequestMagicSkillUse(buildHQSkillID, false, false))
				if !slices.Contains(opcodes(collectUntilQuiet(t, w.c)), serverpackets.OpcodeMagicSkillUse) {
					t.Fatal("the flag cast did not start")
				}
				switch order {
				case "end first":
					end()
					w.srv.Advance(t, 5*time.Second)
				case "flag first":
					w.srv.Advance(t, 5*time.Second)
					end()
				default:
					ended := make(chan struct{})
					go func() {
						defer close(ended)
						end()
					}()
					w.srv.Advance(t, 5*time.Second)
					<-ended
				}
				w.srv.Settle(t)
				if _, ok := w.clan.Headquarters(); ok {
					t.Fatal("the clan holds a headquarters after the siege's end")
				}
				if n := len(w.flags()); n != 0 {
					t.Fatalf("headquarters in the world after the siege's end = %d, want 0", n)
				}
			})
		}
	}
}
