package combat

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestShippedBehindGatedPassivesRaiseCritRateFromBehindOnly lays the shipped
// passive back-blow skills (3018-1, 3019-1 and 3063-1, each a rCrit
// multiplier gated on the player standing behind the target, and 3589-1, a
// flat rCrit bonus under the same gate) on a level-5 player through the
// passive loader. Every critical roll equals the unbuffed rate, so a swing
// crits exactly when the gated term applies: from behind it does, from the
// front it does not.
func TestShippedBehindGatedPassivesRaiseCritRateFromBehindOnly(t *testing.T) {
	t.Parallel()
	skills := shippedSkills(t)
	for _, id := range []modelskill.ID{3018, 3019, 3063, 3589} {
		def, ok := skills.Get(id, 1)
		if !ok {
			t.Fatalf("shipped skill %d-1 missing", id)
		}
		mods, err := effect.PassiveFuncs(*def)
		if err != nil || len(mods) == 0 {
			t.Fatalf("PassiveFuncs(%d-1) = %d mods, err %v, want its stat funcs", id, len(mods), err)
		}
		for _, tc := range []struct {
			name     string
			monster  location.Location
			wantCrit bool
		}{
			{"from behind", posBehind, true},
			{"from the front", posFront, false},
		} {
			t.Run(fmt.Sprintf("%d-1 %s", id, tc.name), func(t *testing.T) {
				t.Parallel()
				srv := gameservertest.Boot(t,
					gameservertest.WithCharacter("Newbie", 5, 0),
					gameservertest.WithWantChars(1),
				)
				c, objID := srv.Client, srv.SoleObjectID(t)
				startInWorld(t, c)
				hostile := srv.SpawnHostileNPCAt(t, tc.monster)
				drainUntilQuiet(t, c)

				obj, ok := srv.State.Player(objID)
				if !ok {
					t.Fatalf("world.Player(%d) missing", objID)
				}
				pc := obj.(interface {
					AddStatFuncs([]effect.Mod)
					CriticalRate() float64
					SetRollSource(func(int) int)
				})
				onQueue(t, srv.PlayerQueue(t, objID), func() {
					unbuffed := int(pc.CriticalRate())
					pc.AddStatFuncs(mods)
					calls := 0
					pc.SetRollSource(func(int) int {
						calls++
						if calls == 2 {
							return unbuffed
						}
						return 0
					})
				})
				drainUntilQuiet(t, c)

				m := tc.monster
				c.Send(encodeAction(hostile.ObjectID(), int32(m.X), int32(m.Y), int32(m.Z), false))
				c.Send(encodeAction(hostile.ObjectID(), int32(m.X), int32(m.Y), int32(m.Z), false))
				hit, _ := firstHitFeedback(t, c, objID)
				if hit.Flags&attack.HitMiss != 0 {
					t.Fatalf("Attack flags = %#x, want a landed hit", hit.Flags)
				}
				if got := hit.Flags&attack.HitCritical != 0; got != tc.wantCrit {
					t.Fatalf("critical = %v (flags %#x), want %v", got, hit.Flags, tc.wantCrit)
				}
			})
		}
	}
}
