// Package skilltest provides a neutral skill.Creature for tests.
package skilltest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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
func (Creature) SkillSuccessInput(creature.FormulaActor, modelskill.Definition, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (Creature) EffectSuccessInput(creature.FormulaActor, modelskill.Definition, modelskill.EffectTemplate, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (Creature) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return formulas.SkillReflectInput{}
}
func (Creature) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
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
func (Creature) PhysicalSkillInput(creature.FormulaActor, modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return formulas.PhysicalSkillInput{}, false
}
func (Creature) MagicDamageInput(creature.FormulaActor, modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return formulas.MagicDamageInput{}, false
}
func (Creature) BlowInput(creature.FormulaActor, modelskill.Definition) (formulas.BlowInput, bool) {
	return formulas.BlowInput{}, false
}
func (Creature) ManaDamageInput(creature.FormulaActor, modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return formulas.ManaDamageInput{}, false
}
func (Creature) LethalInput(creature.FormulaActor, modelskill.Definition) (formulas.LethalInput, bool) {
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

// Formula surface: the neutral creature has no stats, no weapon and no
// shots, so every formula term is the zero value and its rolls are 0.
func (Creature) STR() int                                     { return 0 }
func (Creature) CON() int                                     { return 0 }
func (Creature) DEX() int                                     { return 0 }
func (Creature) INT() int                                     { return 0 }
func (Creature) WIT() int                                     { return 0 }
func (Creature) MEN() int                                     { return 0 }
func (Creature) PAtk() float64                                { return 0 }
func (Creature) PDef() float64                                { return 0 }
func (Creature) MAtk() float64                                { return 0 }
func (Creature) MDef() float64                                { return 0 }
func (Creature) MagicCriticalRate() float64                   { return 0 }
func (Creature) AttackType() item.WeaponType                  { return item.WeaponNone }
func (Creature) SoulshotCharged() bool                        { return false }
func (Creature) SpiritshotCharged() bool                      { return false }
func (Creature) CalcStat(stat.Stat, float64) float64          { return 0 }
func (Creature) RandomDamageSpread() int                      { return 0 }
func (Creature) Roll(int) int                                 { return 0 }
func (Creature) WeaponGradePenalty() bool                     { return false }
func (Creature) Evasion() int                                 { return 0 }
func (Creature) LethalRate() float64                          { return 0 }
func (Creature) RaceMultiplier(creature.FormulaActor) float64 { return 1 }
