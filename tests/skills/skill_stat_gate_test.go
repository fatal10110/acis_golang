package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// skillStatCond is <skill stat="maxHp"/>: it holds only when the skill under
// test names MAX_HP in its "stat" set value.
var skillStatCond = modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "maxHp"}}

// TestEffectAttachConditionGatesItsCreation covers #3669: an <effect>'s
// attached <cond> is tested when the effect is created, with the skill that
// applies it (EffectTemplate.getEffect), and an effect it rejects is never
// created: no icon, no stat funcs. The self-buff's effect carries
// <cond><skill stat="maxHp"/></cond> and a +100 max HP func.
func TestEffectAttachConditionGatesItsCreation(t *testing.T) {
	t.Parallel()
	const skillID = 1204
	for _, tc := range []struct {
		name    string
		stat    string
		created bool
	}{
		{"skill names MAX_HP", "MAX_HP", true},
		{"skill names POWER_ATTACK", "POWER_ATTACK", false},
		{"skill names no stat", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					SkillType: "BUFF", Stat: tc.stat,
					Effects: []modelskill.EffectTemplate{{
						Name: "Buff", Time: 60, Icon: true, StackType: "max_hp_up", StackOrder: 1,
						AttachCondition: &modelskill.ConditionClause{Root: skillStatCond},
						Funcs:           []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 100}},
					}},
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			startInWorld(t, c)
			plain := srv.PlayerMaxHP(t, objID)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
			srv.Advance(t, 700*time.Millisecond)
			frames := collectUntilQuiet(t, c)

			icon := slices.Contains(lastBuffIcons(t, frames), skillID)
			gain := srv.PlayerMaxHP(t, objID) - plain
			if tc.created && (!icon || gain != 100) {
				t.Fatalf("buff icon shown = %v, max HP gain = %d; want the effect created: icon shown, +100", icon, gain)
			}
			if !tc.created && (icon || gain != 0) {
				t.Fatalf("buff icon shown = %v, max HP gain = %d; want no effect: no icon, no gain", icon, gain)
			}
		})
	}
}

// bootPassiveHolder boots a player who knows defs and returns the server,
// the player's object id and the M.Atk its enter-world UserInfo shows.
func bootPassiveHolder(t *testing.T, defs ...modelskill.Definition) (srv *gameservertest.Server, objID int32, mAtk int32) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	objID = srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	}
	frames := startInWorld(t, srv.Client)
	drainUntilQuiet(t, srv.Client)
	at := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeUserInfo })
	if at < 0 {
		t.Fatal("no UserInfo in the enter-world burst")
	}
	r := userInfoCombatReader(t, frames[at])
	for range 6 { // P.Atk, attack speed, P.Def, evasion, accuracy, critical
		r.ReadInt32()
	}
	return srv, objID, r.ReadInt32()
}

// TestPassiveForConditionTestsOwningSkill covers #3669: a <cond> attached to
// a passive skill's <for> block is tested with that skill
// (FuncTemplate.getFunc from L2Skill.getStatFuncs), whatever the stat
// calculation runs for. The passive's +100 max HP func sits under
// <cond><skill stat="maxHp"/></cond>; max HP is calculated for no skill.
func TestPassiveForConditionTestsOwningSkill(t *testing.T) {
	t.Parallel()
	passive := func(stat string) modelskill.Definition {
		return modelskill.Definition{
			ID: 211, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", Stat: stat,
			Funcs: []modelskill.FuncTemplate{{
				Op: modelskill.FuncAdd, Stat: "maxHp", Value: 100,
				AttachCondition: &modelskill.ConditionClause{Root: skillStatCond},
			}},
		}
	}
	// Each boot runs in its own subtest: one test's boots share its account.
	maxHP := map[string]int{}
	for _, stat := range []string{"MAX_HP", ""} {
		t.Run("stat "+stat, func(t *testing.T) {
			srv, objID, _ := bootPassiveHolder(t, passive(stat))
			maxHP[stat] = srv.PlayerMaxHP(t, objID)
		})
	}
	if got, plain := maxHP["MAX_HP"], maxHP[""]; got != plain+100 {
		t.Fatalf("max HP with the passive naming MAX_HP = %d, naming none = %d; want +100", got, plain)
	}
}

// TestFuncConditionTestsCalculationSkill covers #3669: a func's own
// condition is tested with the skill its stat calculation runs for
// (Func.calc): the magic damage formula reads the caster's M.Atk for the
// skill being cast (getMAtk(target, skill)). The caster's passive adds M.Atk
// under <skill stat="mAtk"/>; an MDAM naming MAGIC_ATTACK sees the bonus and
// deals more than one naming no stat, while M.Atk read for no skill does not
// see it. Magic rolls never fail or crit.
func TestFuncConditionTestsCalculationSkill(t *testing.T) {
	t.Parallel()
	passive := modelskill.Definition{
		ID: 212, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		SkillType: "BUFF",
		Funcs: []modelskill.FuncTemplate{{
			Op: modelskill.FuncAdd, Stat: "mAtk", Value: 5000,
			Condition: &modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "mAtk"}},
		}},
	}
	type strike struct{ dealt, mAtk int32 }
	strikes := map[string]strike{}
	for _, stat := range []string{"MAGIC_ATTACK", ""} {
		// Each boot runs in its own subtest: one test's boots share its
		// account.
		t.Run("stat "+stat, func(t *testing.T) {
			srv, objID, mAtk := bootPassiveHolder(t, passive, modelskill.Definition{
				ID: orderSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "MDAM", Power: 10, Magic: true, Stat: stat,
			})
			c := srv.Client
			// An M.Def high enough, and HP deep enough, that neither strike
			// saturates or kills.
			tmpl := conditionNPCTemplate("Monster", 0)
			tmpl.MDef, tmpl.HPMax = 10, 1_000_000
			hostile := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: 60, Y: 20, Z: 30})
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			setCasterMagicRolls(t, srv, objID, func() int { return 500 })

			hp := hostile.CurrentHP()
			c.Send(encodeRequestMagicSkillUse(orderSkill, false, false))
			readCastStartFrames(t, c, objID, orderSkill, 1, 500, 60_000, hostile.ObjectID())
			srv.AdvanceUntil(t, "the strike", func() bool { return hostile.CurrentHP() < hp })
			strikes[stat] = strike{dealt: youDidDamage(t, readFrameLog(c)), mAtk: mAtk}
		})
	}
	named, unnamed := strikes["MAGIC_ATTACK"], strikes[""]
	if named.dealt <= unnamed.dealt {
		t.Fatalf("MDAM damage naming MAGIC_ATTACK = %d, naming none = %d; want the M.Atk bonus to raise it", named.dealt, unnamed.dealt)
	}
	if named.mAtk != unnamed.mAtk {
		t.Fatalf("M.Atk read for no skill = %d and %d; want the same, without the bonus", named.mAtk, unnamed.mAtk)
	}
}
