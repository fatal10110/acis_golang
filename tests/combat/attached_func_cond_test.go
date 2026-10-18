package combat

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEffectFuncAttachedConditionFixedAtAttach pins #3862: the <cond>
// attached to an effect's func group is tested once, when the effect's
// funcs are attached (FuncTemplate.getFunc from AbstractEffect.getStatFuncs),
// against the effect's effector and effected; a func it rejects is not
// attached, and a later change of the tested state does not attach or drop
// it. The func adds 100 P.Atk. under <player hp="50"/>.
func TestEffectFuncAttachedConditionFixedAtAttach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		lowFirst bool
		wantGain float64
	}{
		{"attached at full HP", false, 0},
		{"attached at low HP", true, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatalf("world.Player(%d) missing", objID)
			}
			pc := obj.(interface {
				effectHolder
				PAtk() float64
				SetHP(float64)
			})
			maxHP := float64(srv.PlayerMaxHP(t, objID))
			var base, attached, flipped float64
			onQueue(t, srv.PlayerQueue(t, objID), func() {
				base = pc.PAtk()
				if tc.lowFirst {
					pc.SetHP(maxHP * 3 / 10)
				}
				effect.Apply(pc, pc, effect.Skill{ID: 4242, Level: 1, SkillType: "BUFF"}, []modelskill.EffectTemplate{{
					Name: "Buff", Time: 600,
					Funcs: []modelskill.FuncTemplate{{
						Op: modelskill.FuncAdd, Stat: "pAtk", Value: 100,
						AttachCondition: &modelskill.ConditionClause{Root: modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": "50"}}},
					}},
				}})
				attached = pc.PAtk()
				if tc.lowFirst {
					pc.SetHP(maxHP)
				} else {
					pc.SetHP(maxHP * 3 / 10)
				}
				flipped = pc.PAtk()
			})
			if got := attached - base; got != tc.wantGain {
				t.Fatalf("P.Atk. gain once attached = %v, want %v", got, tc.wantGain)
			}
			if flipped != attached {
				t.Fatalf("P.Atk. after the HP change = %v, want it fixed at %v", flipped, attached)
			}
		})
	}
}
