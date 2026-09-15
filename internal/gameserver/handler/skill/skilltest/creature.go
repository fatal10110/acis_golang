// Package skilltest provides a neutral skill.Creature for tests.
package skilltest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Creature supplies neutral values for every skill.Creature method except
// the world placement ones: embed it next to world.Presence in a test double
// and override only what the test exercises. The neutral creature cannot be
// rolled against, never reflects or blocks, and is not an NPC.
type Creature struct {
	targettest.Actor
}

func (Creature) Invul() bool                    { return false }
func (Creature) Paralyzed() bool                { return false }
func (Creature) BlessedSpiritshotCharged() bool { return false }
func (Creature) SkillSuccessInput(attackable.Combatant, modelskill.Definition, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (Creature) EffectSuccessInput(attackable.Combatant, modelskill.Definition, modelskill.EffectTemplate, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (Creature) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return formulas.SkillReflectInput{}
}
func (Creature) ShieldDefense(attackable.Combatant, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}
func (Creature) Attackable() bool                           { return false }
func (Creature) NotifyAggression(attackable.Combatant, int) {}
func (Creature) ReduceAllAggroHate(float64)                 {}
func (Creature) StopAggroHate(attackable.Combatant)         {}
func (Creature) StopHateList(attackable.Combatant)          {}
func (Creature) ClearAggroTables()                          {}
func (Creature) EnableOverhit()                             {}
func (Creature) CurrentTarget() world.Tracked               { return nil }
func (Creature) SetTarget(world.Tracked)                    {}
func (Creature) AttackTarget(world.Tracked)                 {}
