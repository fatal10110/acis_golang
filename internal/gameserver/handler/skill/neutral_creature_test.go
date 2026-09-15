package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// neutralCreature supplies neutral values for every Creature method except
// the world placement ones: embed it next to world.Presence in a test double
// and override only what the test exercises. The neutral creature cannot be
// rolled against, never reflects or blocks, and is not an NPC.
type neutralCreature struct {
	targettest.Actor
}

func (neutralCreature) Invul() bool                    { return false }
func (neutralCreature) Paralyzed() bool                { return false }
func (neutralCreature) BlessedSpiritshotCharged() bool { return false }
func (neutralCreature) SkillSuccessInput(attackable.Combatant, modelskill.Definition, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (neutralCreature) EffectSuccessInput(attackable.Combatant, modelskill.Definition, modelskill.EffectTemplate, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{}, false
}
func (neutralCreature) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return formulas.SkillReflectInput{}
}
func (neutralCreature) ShieldDefense(attackable.Combatant, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}
func (neutralCreature) Attackable() bool                           { return false }
func (neutralCreature) NotifyAggression(attackable.Combatant, int) {}
func (neutralCreature) ReduceAllAggroHate(float64)                 {}
func (neutralCreature) StopAggroHate(attackable.Combatant)         {}
func (neutralCreature) StopHateList(attackable.Combatant)          {}
func (neutralCreature) ClearAggroTables()                          {}
func (neutralCreature) EnableOverhit()                             {}
func (neutralCreature) CurrentTarget() world.Tracked               { return nil }
func (neutralCreature) SetTarget(world.Tracked)                    {}
func (neutralCreature) AttackTarget(world.Tracked)                 {}
