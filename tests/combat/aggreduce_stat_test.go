package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestZeroPowerAggReduceReducesByAggressionStat pins #3863: an AGGREDUCE
// with no power reduces every hate entry by how far the target's AGGRESSION
// stat, calculated for the cast skill from its hate against the caster,
// lowers that hate (Disablers AGGREDUCE). The monster holds a halving
// AGGRESSION passive func gated on <skill stat="aggression">, so it applies only to
// a calculation run for a skill naming that stat: the reduction is half the
// caster's hate, taken from every entry. A skill naming another stat leaves
// the hate alone. A bystander's hate shows the reduction: the cast itself
// adds no hate of its own against the bystander.
func TestZeroPowerAggReduceReducesByAggressionStat(t *testing.T) {
	t.Parallel()
	const aggReduceID = 1068
	for _, tc := range []struct {
		name      string
		skillStat string
		want      float64
	}{
		{"cast for the aggression stat", "AGGRESSION", 600},
		{"cast for another stat", "MAX_HP", 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Reducer", 10, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{{
					ID: aggReduceID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
					SkillType: "AGGREDUCE", Offensive: true, Stat: tc.skillStat,
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, aggReduceID, 1)
			startInWorld(t, c)
			bystanderID := srv.SeedCharacterFor(t, "bystander", "Bystander", 10, 0).ID
			bc := srv.DialClient(t, "bystander", 1)
			startInWorld(t, bc)
			monster := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, bc)
			drainUntilQuiet(t, c)
			combatant := func(id int32) attackable.Combatant {
				obj, ok := srv.State.Player(id)
				if !ok {
					t.Fatalf("world.Player(%d) missing", id)
				}
				return obj.(attackable.Combatant)
			}
			caster, other := combatant(objID), combatant(bystanderID)
			onQueue(t, monster.Queue(), func() {
				monster.AddDamageHate(caster, 0, 800)
				monster.AddDamageHate(other, 0, 1000)
				mods, err := effect.SkillStatFuncs(modelskill.Definition{ID: 4242, Level: 1, Funcs: []modelskill.FuncTemplate{{
					Op: modelskill.FuncMul, Stat: "aggression", Value: 0.5,
					Condition: &modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "aggression"}},
				}}})
				if err != nil {
					t.Errorf("SkillStatFuncs: %v", err)
					return
				}
				monster.AddStatFuncs(mods)
			})

			targetHostile(t, c, monster.ObjectID())
			drainUntilQuiet(t, c)
			c.Send(encodeRequestMagicSkillUse(aggReduceID, false, false))
			readCastStartFrames(t, c, objID, aggReduceID, 1, 500, 0, monster.ObjectID())
			srv.Advance(t, time.Second)
			drainUntilQuiet(t, c)

			var got float64
			onQueue(t, monster.Queue(), func() { got = monster.AggroHate(other) })
			if got != tc.want {
				t.Fatalf("other attacker's hate after the cast = %v, want %v", got, tc.want)
			}
		})
	}
}
