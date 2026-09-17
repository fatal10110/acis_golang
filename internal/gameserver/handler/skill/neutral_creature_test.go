package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

// Damage and resource surface: the neutral creature takes no damage, rolls
// no formula input and holds no resources.
func (neutralCreature) ReduceHP(float64, attackable.Combatant, modelskill.Definition) {}
func (neutralCreature) PhysicalSkillInput(attackable.Combatant, modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return formulas.PhysicalSkillInput{}, false
}
func (neutralCreature) MagicDamageInput(attackable.Combatant, modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return formulas.MagicDamageInput{}, false
}
func (neutralCreature) BlowInput(attackable.Combatant, modelskill.Definition) (formulas.BlowInput, bool) {
	return formulas.BlowInput{}, false
}
func (neutralCreature) ManaDamageInput(attackable.Combatant, modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return formulas.ManaDamageInput{}, false
}
func (neutralCreature) LethalInput(attackable.Combatant, modelskill.Definition) (formulas.LethalInput, bool) {
	return formulas.LethalInput{}, false
}
func (neutralCreature) ApplyLethalOutcome(formulas.LethalOutcome, attackable.Combatant, modelskill.Definition) {
}
func (neutralCreature) CounterSkillPhysical() float64 { return 0 }
func (neutralCreature) Invulnerable() bool            { return false }
func (neutralCreature) HealAmount(modelskill.Definition) (float64, bool) {
	return 0, false
}
func (neutralCreature) MaxHPValue() float64 { return 0 }
func (neutralCreature) MaxMPValue() float64 { return 0 }
func (neutralCreature) SetHP(float64)       {}

// neutralPlayer adds neutral values for the player-only cast surface on top
// of neutralCreature, so a player-kind double only overrides what its test
// exercises.
type neutralPlayer struct {
	neutralCreature
}

func (neutralPlayer) Kind() actor.Kind                               { return actor.KindPlayer }
func (neutralPlayer) CP() float64                                    { return 0 }
func (neutralPlayer) MaxCPValue() float64                            { return 0 }
func (neutralPlayer) SetCP(float64)                                  {}
func (neutralPlayer) BreakCastOnDamage(float64)                      {}
func (neutralPlayer) Charges() int                                   { return 0 }
func (neutralPlayer) Revive(float64) bool                            { return false }
func (neutralPlayer) RestoreExp(float64)                             {}
func (neutralPlayer) CursedWeaponEquipped() bool                     { return false }
func (neutralPlayer) Operating() bool                                { return false }
func (neutralPlayer) Rooted() bool                                   { return false }
func (neutralPlayer) InCombat() bool                                 { return false }
func (neutralPlayer) FestivalParticipant() bool                      { return false }
func (neutralPlayer) NotifyHPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyMPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyCPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyAttackFailed()                            {}
func (neutralPlayer) NotifyResistedSkill(string, modelskill.ID, int) {}
func (neutralPlayer) NotifyResistedMagic(string)                     {}
func (neutralPlayer) NotifySpoilAlready()                            {}
func (neutralPlayer) NotifySpoilSuccess()                            {}
func (neutralPlayer) Mounted() bool                                  { return false }
func (neutralPlayer) OlympiadMode() bool                             { return false }
func (neutralPlayer) ObserverMode() bool                             { return false }
func (neutralPlayer) NoSummonFriendZone() bool                       { return false }

// neutralNPC adds the NPC-only cast surface on top of neutralCreature.
type neutralNPC struct {
	neutralCreature
}

func (neutralNPC) Kind() actor.Kind           { return actor.KindNPC }
func (neutralNPC) Lethalable() bool           { return true }
func (neutralNPC) SpoilPool() *item.SpoilPool { return nil }
