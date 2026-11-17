package skills

import (
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const flagStrikeSkillID = 9350

// flagStrike is an offensive single-target skill: a physical damage hit
// carrying a debuff effect when damaging, or the debuff alone.
func flagStrike(skillType string) modelskill.Definition {
	return modelskill.Definition{
		ID: flagStrikeSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, SkillType: skillType, Power: 500, Offensive: true,
		CastRange: 900, HitTime: 500, StaticHitTime: true,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 10}},
	}
}

// TestOffensiveSkillOnSiegeFlag casts an offensive single-target skill, with
// no CTRL pressed, at an enemy headquarters. A siege flag is no Folk or
// Guard, so the target gate lets any offensive skill through (a folk takes
// only a CTRL-pressed damage skill); a damage skill lands its hit, and
// whatever the skill, no effect lands on the flag (L2Skill.getEffects).
func TestOffensiveSkillOnSiegeFlag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		skillType  string
		wantDamage bool
	}{
		{skillType: "PDAM", wantDamage: true},
		{skillType: "DEBUFF"},
	} {
		t.Run(tc.skillType, func(t *testing.T) {
			t.Parallel()
			def := flagStrike(tc.skillType)
			w := bootHQ(t, hqSetup{known: []modelskill.Definition{def}})
			flag := w.otherFlag(t, "Rivals", 100)
			selectTarget(t, w.c, flag.ObjectID())
			drainUntilQuiet(t, w.c)
			before := flag.CurrentHP()

			w.c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			frames := collectUntilQuiet(t, w.c)
			w.srv.Advance(t, 2*time.Second)
			frames = append(frames, collectUntilQuiet(t, w.c)...)
			if !slices.Contains(opcodes(frames), serverpackets.OpcodeMagicSkillUse) {
				t.Fatalf("cast at the flag answered %x, want it started", opcodes(frames))
			}
			for _, frame := range frames {
				if frame[0] == serverpackets.OpcodeSystemMessage && wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageInvalidTarget {
					t.Fatal("an offensive skill at a siege flag answered INVALID_TARGET")
				}
			}
			if damaged := flag.CurrentHP() < before; damaged != tc.wantDamage {
				t.Fatalf("flag hp %d -> %d, damaged = %v, want %v", before, flag.CurrentHP(), damaged, tc.wantDamage)
			}
			if n := len(flag.EffectList().All()); n != 0 {
				t.Fatalf("the flag carries %d effects, want none", n)
			}
		})
	}
}
