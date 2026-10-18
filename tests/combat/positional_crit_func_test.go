package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBehindGatedCritFuncTestsAttackTarget pins #3861 with the shipped
// Prophecy of Wind (1357-1): its <basemul stat="rCrit" val="0.2"><player
// behind="True"/></basemul> raises the critical rate of a swing landed from
// behind the target (Func.calc tests the attack target, CreatureAttack
// passes it to getCriticalHit), while the critical rate a status packet
// shows, calculated against no target, stays unbuffed. Every critical roll
// here equals that unbuffed rate, so a swing crits exactly when the bonus
// applies. The fixture monster faces east (heading 0); the player stands at
// x=10.
func TestBehindGatedCritFuncTestsAttackTarget(t *testing.T) {
	t.Parallel()
	def, ok := shippedSkills(t).Get(1357, 1)
	if !ok {
		t.Fatal("shipped skill 1357-1 missing")
	}
	for _, tc := range []struct {
		name     string
		monsterX int
		buffed   bool
		wantCrit bool
	}{
		{"buffed, from behind", 30, true, true},
		{"buffed, from the front", -10, true, false},
		{"unbuffed, from behind", 30, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: tc.monsterX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatalf("world.Player(%d) missing", objID)
			}
			pc := obj.(interface {
				effectHolder
				CriticalRate() float64
				SetRollSource(func(int) int)
			})
			var unbuffed, shown float64
			onQueue(t, srv.PlayerQueue(t, objID), func() {
				unbuffed = pc.CriticalRate()
				if tc.buffed {
					effect.Apply(pc, pc, effect.SkillFromDefinition(def), def.Effects)
				}
				shown = pc.CriticalRate()
				// The hit roll lands, the critical roll sits at the unbuffed rate.
				calls := 0
				pc.SetRollSource(func(int) int {
					calls++
					if calls == 2 {
						return int(unbuffed)
					}
					return 0
				})
			})
			if shown != unbuffed {
				t.Fatalf("critical rate against no target = %v with the buff, want the unbuffed %v", shown, unbuffed)
			}
			drainUntilQuiet(t, c)

			c.Send(encodeAction(hostile.ObjectID(), int32(tc.monsterX), hostileY, hostileZ, false))
			c.Send(encodeAction(hostile.ObjectID(), int32(tc.monsterX), hostileY, hostileZ, false))
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
