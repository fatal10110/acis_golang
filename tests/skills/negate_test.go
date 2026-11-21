package skills

import (
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Seed skills a negate scenario lands on its caster before negating.
const (
	negSeedWeakPoison     = 3101 // POISON, abnormal level 2
	negSeedStrongPoison   = 3102 // POISON, abnormal level 4
	negSeedPoisonInDebuff = 3103 // DEBUFF carrying a POISON effect type at effect abnormal level 2
	negSeedBleedInPoison  = 3104 // POISON carrying a BLEED effect type at effect abnormal level 1
	negSeedParalyzeTagged = 3105 // DEBUFF whose skill-level effectType is PARALYZE, template untyped
	negSeedUndispellable  = 3106 // POISON, abnormal level 1, stack order 99
)

// negateSeed is a self-cast debuff with an icon, its own stack type, and
// the given classification and abnormal levels.
func negateSeed(id modelskill.ID, skillType, effectType string, abnormal, effectAbnormal int, stackOrder float64) modelskill.Definition {
	return modelskill.Definition{
		ID:            id,
		Level:         1,
		Activation:    modelskill.ActivationActive,
		Target:        modelskill.TargetSelf,
		StaticHitTime: true,
		SkillType:     skillType,
		Debuff:        true,
		AbnormalLevel: abnormal,
		Effects: []modelskill.EffectTemplate{{
			Name: "Debuff", Time: 60, Icon: true,
			StackType: "negate_seed_" + strconv.Itoa(int(id)), StackOrder: stackOrder,
		}},
	}.WithExtra(modelskill.Extra{
		EffectType:          effectType,
		EffectAbnormalLevel: effectAbnormal,
		NegateLevel:         -1,
	})
}

// negateSkill is a self-cast NEGATE with no effects of its own.
func negateSkill(id modelskill.ID, level int, types []string, ids []int) modelskill.Definition {
	return modelskill.Definition{
		ID:            id,
		Level:         1,
		Activation:    modelskill.ActivationActive,
		Target:        modelskill.TargetSelf,
		StaticHitTime: true,
		SkillType:     "NEGATE",
		AbnormalLevel: -1,
	}.WithExtra(modelskill.Extra{
		EffectAbnormalLevel: -1,
		NegateLevel:         level,
		NegateTypes:         types,
		NegateIDs:           ids,
	})
}

// TestNegateStripsSeededEffects lands a set of debuffs on a player, casts
// one NEGATE, and checks which effects survive, both in the player's effect
// list and in the icon list the client last received.
//
// A level-gated negate strips a candidate by its effect type and effect
// abnormal level when the candidate's skill has an effect abnormal level,
// else by its skill type and abnormal level; a -1 negate strips any level
// by either type, the effect type resolved from the skill when its template
// carries none. Neither touches a stack-order-99 effect. A negate-by-id list
// strips only those skills, ignoring its negate types.
func TestNegateStripsSeededEffects(t *testing.T) {
	seeds := []modelskill.Definition{
		negateSeed(negSeedWeakPoison, "POISON", "", 2, -1, 1),
		negateSeed(negSeedStrongPoison, "POISON", "", 4, -1, 1),
		negateSeed(negSeedPoisonInDebuff, "DEBUFF", "POISON", 9, 2, 1),
		negateSeed(negSeedBleedInPoison, "POISON", "BLEED", 1, 1, 1),
		negateSeed(negSeedParalyzeTagged, "DEBUFF", "PARALYZE", -1, -1, 1),
		negateSeed(negSeedUndispellable, "POISON", "", 1, -1, 99),
	}
	all := []int{negSeedWeakPoison, negSeedStrongPoison, negSeedPoisonInDebuff, negSeedBleedInPoison, negSeedParalyzeTagged, negSeedUndispellable}
	for _, tc := range []struct {
		name   string
		negate modelskill.Definition
		want   []int
	}{
		{
			name:   "poison level 3",
			negate: negateSkill(3201, 3, []string{"POISON"}, nil),
			want:   []int{negSeedStrongPoison, negSeedBleedInPoison, negSeedParalyzeTagged, negSeedUndispellable},
		},
		{
			name:   "poison any level",
			negate: negateSkill(3201, -1, []string{"POISON"}, nil),
			want:   []int{negSeedParalyzeTagged, negSeedUndispellable},
		},
		{
			name:   "paralyze any level",
			negate: negateSkill(3201, -1, []string{"PARALYZE"}, nil),
			want:   []int{negSeedWeakPoison, negSeedStrongPoison, negSeedPoisonInDebuff, negSeedBleedInPoison, negSeedUndispellable},
		},
		{
			name:   "by id",
			negate: negateSkill(3201, -1, []string{"POISON"}, []int{negSeedStrongPoison, negSeedUndispellable}),
			want:   []int{negSeedWeakPoison, negSeedPoisonInDebuff, negSeedBleedInPoison, negSeedParalyzeTagged},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, append(slices.Clone(seeds), tc.negate))),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			for _, id := range all {
				seedKnownSkill(t, srv, objID, id, 1)
			}
			seedKnownSkill(t, srv, objID, int(tc.negate.ID), 1)
			startInWorld(t, c)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetRollSource(func(int) int { return 0 }) })

			for i, id := range all {
				castSelfUntilIcons(t, c, int32(id), all[:i+1])
			}
			if got := negateEffectSkills(t, srv, objID); !slices.Equal(got, all) {
				t.Fatalf("seeded effects = %v, want %v", got, all)
			}
			drainUntilQuiet(t, c)

			castSelfUntilIcons(t, c, int32(tc.negate.ID), tc.want)
			if got := negateEffectSkills(t, srv, objID); !slices.Equal(got, tc.want) {
				t.Fatalf("effects after negate = %v, want %v", got, tc.want)
			}
		})
	}
}

// castSelfUntilIcons casts skillID and reads frames until the client receives
// an AbnormalStatusUpdate listing exactly the skill ids in want. That frame is
// the cast's client-visible result: a launch frame only says the cast began,
// and on the pool executor the hit that applies or strips effects can land
// after it. Earlier icon lists of the same cast are skipped.
func castSelfUntilIcons(t *testing.T, c *testsupport.ScriptedClient, skillID int32, want []int) {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	var last []int
	for range 20 {
		frame := c.Read()
		if frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
			continue
		}
		last = last[:0]
		for _, e := range readAbnormalStatusUpdateEntriesFromFrame(t, frame) {
			last = append(last, int(e.SkillID))
		}
		slices.Sort(last)
		if slices.Equal(last, want) {
			return
		}
	}
	t.Fatalf("cast of skill %d never sent an AbnormalStatusUpdate listing %v; last icons = %v", skillID, want, last)
}

// negateEffectSkills returns the sorted skill ids of the player's live
// effects, read on the player's own queue after every queued action.
func negateEffectSkills(t *testing.T, srv *gameservertest.Server, objID int32) []int {
	t.Helper()
	var ids []int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		for _, e := range pc.EffectList().All() {
			ids = append(ids, int(e.Skill.ID))
		}
	})
	slices.Sort(ids)
	return slices.Compact(ids)
}
