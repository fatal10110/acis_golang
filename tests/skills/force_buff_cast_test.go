package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	forceFusionID      int32 = 3630
	battleForceID      int32 = 5104
	needsBattleForce1  int32 = 3631
	needsBattleForce2  int32 = 3632
	needsSpellForce1ID int32 = 3633
)

// forceGatedSkill is a free self-cast whose <cond> needs the given force
// attribute (battle_force or spell_force) at level.
func forceGatedSkill(id int32, attr, level string) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "BUFF",
		Conditions: []modelskill.ConditionClause{{
			Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{attr: level}},
			MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
		}},
	}
}

// TestForceBuffCastConditionsReadFusionLevel has Caster open a FUSION channel
// on Mate. Mate's own skills gated on <player battle_force="N"/> refuse
// before the channel and below level N, and start once the Battle Force the
// fusion landed on Mate reaches N; a spell_force gate keeps refusing because
// the fusion carries only Battle Force.
func TestForceBuffCastConditionsReadFusionLevel(t *testing.T) {
	t.Parallel()
	fusion := modelskill.Definition{
		ID: modelskill.ID(forceFusionID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 15_000, StaticHitTime: true, SkillType: "FUSION",
		TriggeredID: int(battleForceID), TriggeredLevel: 1,
	}
	force := modelskill.Definition{
		ID: modelskill.ID(battleForceID), Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Fusion", Time: 600, Icon: true}},
	}
	mateDefs := []modelskill.Definition{
		force,
		forceGatedSkill(needsBattleForce1, "battle_force", "1"),
		forceGatedSkill(needsBattleForce2, "battle_force", "2"),
		forceGatedSkill(needsSpellForce1ID, "spell_force", "1"),
	}
	s := bootSocialSceneKnowing(t, fusion, mateDefs)
	s.formParty(t)

	for _, id := range []int32{needsBattleForce1, needsBattleForce2, needsSpellForce1ID} {
		if castStarts(t, s.mate, id) {
			t.Fatalf("skill %d started before any force", id)
		}
	}

	s.selectPlayer(t, s.mateID)
	s.caster.Send(encodeRequestMagicSkillUse(forceFusionID, false, false))
	for frame := s.caster.Read(); frame[0] != serverpackets.OpcodeMagicSkillUse; frame = s.caster.Read() {
		if frame[0] == serverpackets.OpcodeSystemMessage || frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("the fusion cast on Mate was refused with frame %#x", frame[0])
		}
	}
	s.quiet(t)

	for _, tc := range []struct {
		id   int32
		want bool
	}{{needsBattleForce2, false}, {needsSpellForce1ID, false}, {needsBattleForce1, true}} {
		if got := castStarts(t, s.mate, tc.id); got != tc.want {
			t.Fatalf("with Battle Force level 1: skill %d started = %v, want %v", tc.id, got, tc.want)
		}
	}
}
