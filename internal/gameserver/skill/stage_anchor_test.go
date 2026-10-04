package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// A subclass change stages the new class's saved effects for a character
// already in the world, right before replaying them: they carry no restore
// instant, so each runs from the replay itself and the replay runs none of
// their ticks, however long it takes. Only a login's restore anchors them
// at selection.
func TestStageClassSkillStateAnchorsEffectsAtTheReplay(t *testing.T) {
	poison := modelskill.Definition{ID: 84, Level: 1, Effects: []modelskill.EffectTemplate{
		{Name: "DamOverTime", Count: 10, Time: 3, Value: 5, StackType: "poison", StackOrder: 1},
	}}
	p := NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{poison}))
	c := &player.Character{ID: 1}

	p.StageClassSkillState(c, ClassSkills{saved: []effect.SaveRow{{
		Skill: modelskill.Ref{ID: 84, Level: 1}, EffectCount: 10, EffectCurTime: 3, RestoreType: effect.RestoreTypeEffect,
	}}})

	staged := c.ActiveSkillEffects()
	if len(staged) != 1 {
		t.Fatalf("staged effects = %d, want the poison", len(staged))
	}
	if !staged[0].RestoredAt.IsZero() {
		t.Fatalf("staged poison restore instant = %v, want none: it runs from the replay", staged[0].RestoredAt)
	}
}
