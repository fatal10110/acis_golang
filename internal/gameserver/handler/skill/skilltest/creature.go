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

// Damage and resource surface: the neutral creature takes no damage, rolls
// no formula input and holds no resources.
func (Creature) ReduceHP(float64, attackable.Combatant, modelskill.Definition) {}
func (Creature) PhysicalSkillInput(attackable.Combatant, modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return formulas.PhysicalSkillInput{}, false
}
func (Creature) MagicDamageInput(attackable.Combatant, modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return formulas.MagicDamageInput{}, false
}
func (Creature) BlowInput(attackable.Combatant, modelskill.Definition) (formulas.BlowInput, bool) {
	return formulas.BlowInput{}, false
}
func (Creature) ManaDamageInput(attackable.Combatant, modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return formulas.ManaDamageInput{}, false
}
func (Creature) LethalInput(attackable.Combatant, modelskill.Definition) (formulas.LethalInput, bool) {
	return formulas.LethalInput{}, false
}
func (Creature) ApplyLethalOutcome(formulas.LethalOutcome, attackable.Combatant, modelskill.Definition) {
}
func (Creature) CounterSkillPhysical() float64 { return 0 }
func (Creature) Invulnerable() bool            { return false }
func (Creature) HealAmount(modelskill.Definition) (float64, bool) {
	return 0, false
}
func (Creature) MaxHPValue() float64 { return 0 }
func (Creature) MaxMPValue() float64 { return 0 }
func (Creature) SetHP(float64)       {}
