package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The skill reads and the direct effect landing the freeze of the
// script-facing API adds (#3748).

const (
	freezeBuffID  = modelskill.ID(9421)
	freezeMagicID = modelskill.ID(9422)
)

func freezeSkills() gameservertest.Option {
	return gameservertest.WithScriptSkills(
		modelskill.Definition{
			ID: freezeBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 900, HitTime: 500,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "freeze_probe", StackOrder: 1}},
		},
		modelskill.Definition{
			ID: freezeMagicID, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "MDAM", Magic: true, Element: modelskill.ElementFire, EffectID: 4072, Power: 50,
		},
	)
}

// TestScriptSkillReadsAndDirectEffects pins L2Skill.getEffectId,
// getElement and isMagic, and L2Skill.getEffects(effector, effected) (the
// curse Q365 lands from its NPC): the skill's effects land on the player at
// once, with no cast shown; a skill the table lacks aborts the hook.
func TestScriptSkillReadsAndDirectEffects(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, freezeSkills())
	x, y, z := srv.PlayerPosition(t, objID)
	monster := script.NPCOf(srv.SpawnHostileNPCAt(t, location.Location{X: x + 40, Y: y, Z: z}))
	srv.ReadQueued(t, srv.Client)

	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		k := sc.Skill(modelskill.Ref{ID: freezeMagicID, Level: 2})
		if k.EffectID() != 4072 || k.Element() != modelskill.ElementFire || !k.Magic() {
			t.Errorf("EffectID %d, Element %v, Magic %v; want 4072, fire, true", k.EffectID(), k.Element(), k.Magic())
		}
		if sc.Skill(modelskill.Ref{ID: freezeBuffID, Level: 1}).Magic() {
			t.Error("the buff reads as magic")
		}
		sc.ApplyEffects(modelskill.Ref{ID: freezeBuffID, Level: 1}, monster, p)
	})
	srv.AdvanceUntil(t, "the buff landed on the player", func() bool {
		var held bool
		run(t, srv, objID, func(_ *script.Script, p *script.Player) { held = p.HasEffect(int32(freezeBuffID)) })
		return held
	})
	if got := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeMagicSkillUse); len(got) != 0 {
		t.Fatalf("a direct effect landing showed %d MagicSkillUse", len(got))
	}
	if srv.RunScript(t, objID, q001, func(sc *script.Script, p *script.Player) {
		sc.ApplyEffects(modelskill.Ref{ID: 4242, Level: 1}, p, p)
	}) {
		t.Fatal("ApplyEffects of a skill the table does not hold returned")
	}
}
