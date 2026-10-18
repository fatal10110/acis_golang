package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	critTargetFolk = 30120
	condFuncFolk   = 30121
)

// TestBehindGatedCritFuncAgainstFolkTarget pins #3881 on the target side:
// a civilian NPC presents a condition view, so a positional rCrit func
// (ConditionPlayerState BEHIND/FRONT tests the attacker against the real
// target) follows where a forced swing at it lands from. Prophecy of Wind
// (1357-1) adds 20% from behind only; Focus Chance (356-1) takes 30% off
// from the front and adds 30% from the side. Every critical roll equals
// the unbuffed rate, so a swing crits exactly when the net bonus is
// positive. The NPC faces east (heading 0); the player stands at x=10.
func TestBehindGatedCritFuncAgainstFolkTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		folkX    int
		buff     modelskill.ID
		wantCrit bool
	}{
		{"Prophecy of Wind, from behind", 30, 1357, true},
		{"Prophecy of Wind, from the front", -10, 1357, false},
		{"Focus Chance, from the front", -10, 356, false},
		{"unbuffed, from behind", 30, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			at := location.Location{X: tc.folkX, Y: hostileY, Z: hostileZ}
			folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", critTargetFolk), at)
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
			var def modelskill.Definition
			if tc.buff != 0 {
				if def, ok = shippedSkills(t).Get(tc.buff, 1); !ok {
					t.Fatalf("shipped skill %d-1 missing", tc.buff)
				}
			}
			onQueue(t, srv.PlayerQueue(t, objID), func() {
				unbuffed := pc.CriticalRate()
				if tc.buff != 0 {
					effect.Apply(pc, pc, effect.SkillFromDefinition(def), def.Effects)
				}
				calls := 0
				pc.SetRollSource(func(int) int {
					calls++
					if calls == 2 {
						return int(unbuffed)
					}
					return 0
				})
			})
			drainUntilQuiet(t, c)

			c.Send(encodeAction(folk.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
			drainUntilQuiet(t, c)
			c.Send(encodeAttackRequest(folk.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
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

// TestFolkOwnFuncConditionsTested pins #3881 on the NPC's own funcs: a
// civilian NPC runs its stat calculation with itself as the effector, so a
// func gated by its own condition follows the NPC's live state, a func
// with an attached <cond> attaches when it holds at attach time, and a
// positional func tests the NPC against the creature its swing targets.
func TestFolkOwnFuncConditionsTested(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	// The player stands at x=10 facing east; the NPC stands west of it,
	// behind it.
	tmpl := gameservertest.FolkTemplate("Folk", condFuncFolk)
	tmpl.PAtk, tmpl.MAtk, tmpl.CritRate = 200, 150, 40
	folk := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: -10, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	pc := obj.(interface {
		attackable.Combatant
		conditions.Source
		SetHeading(int)
	})
	onQueue(t, srv.PlayerQueue(t, objID), func() { pc.SetHeading(0) })

	buff := func(fn modelskill.FuncTemplate) []modelskill.EffectTemplate {
		return []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Funcs: []modelskill.FuncTemplate{fn}}}
	}
	hpHalf := modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": "50"}}
	maxHP := folk.MaxHPValue()

	t.Run("own condition follows live state", func(t *testing.T) {
		var base, full, low float64
		onQueue(t, folk.Queue(), func() {
			base = folk.PAtk()
			effect.Apply(folk, folk, effect.Skill{ID: 4242, Level: 1, SkillType: "BUFF"}, buff(modelskill.FuncTemplate{
				Op: modelskill.FuncAdd, Stat: "pAtk", Value: 100, Condition: &hpHalf,
			}))
			full = folk.PAtk()
			folk.SetHP(maxHP * 3 / 10)
			low = folk.PAtk()
			folk.SetHP(maxHP)
		})
		if full != base {
			t.Fatalf("P.Atk. at full HP = %v, want the unbuffed %v", full, base)
		}
		if low != base+100 {
			t.Fatalf("P.Atk. at 30%% HP = %v, want %v", low, base+100)
		}
	})

	t.Run("attached condition tested at attach", func(t *testing.T) {
		var base, attached float64
		onQueue(t, folk.Queue(), func() {
			base = folk.MAtk()
			folk.SetHP(maxHP * 3 / 10)
			effect.Apply(folk, folk, effect.Skill{ID: 4243, Level: 1, SkillType: "BUFF"}, buff(modelskill.FuncTemplate{
				Op: modelskill.FuncAdd, Stat: "mAtk", Value: 100,
				AttachCondition: &modelskill.ConditionClause{Root: hpHalf},
			}))
			folk.SetHP(maxHP)
			attached = folk.MAtk()
		})
		if attached != base+100 {
			t.Fatalf("M.Atk. once attached at 30%% HP = %v, want %v", attached, base+100)
		}
	})

	t.Run("positional func tests the swing target", func(t *testing.T) {
		def, ok := shippedSkills(t).Get(1357, 1)
		if !ok {
			t.Fatal("shipped skill 1357-1 missing")
		}
		var hit attack.Hit
		var unbuffed, shown, against float64
		onQueue(t, folk.Queue(), func() {
			unbuffed = folk.CriticalRate()
			effect.Apply(folk, folk, effect.SkillFromDefinition(def), def.Effects)
			shown = folk.CriticalRate()
			against = folk.Against(pc.ConditionActor(), nil).CriticalRate()
			calls := 0
			folk.SetRollSource(func(int) int {
				calls++
				if calls == 2 {
					return int(unbuffed)
				}
				return 0
			})
			hit = folk.MakeAttackHit(pc, false)
			folk.SetRollSource(nil)
		})
		if shown != unbuffed {
			t.Fatalf("critical rate against no target = %v with the buff, want the unbuffed %v", shown, unbuffed)
		}
		if against <= unbuffed {
			t.Fatalf("critical rate from behind the player = %v, want above the unbuffed %v", against, unbuffed)
		}
		if hit.Miss || !hit.Crit {
			t.Fatalf("swing from behind = %+v, want a landed critical hit", hit)
		}
	})
}
