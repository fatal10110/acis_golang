// Package doorhit gives a door the creature surface a hit lands through: an
// auto-attack's hit and damage roll, and a damage skill's formula inputs.
//
// A door only ever takes hits. It holds no effects, never attacks, never
// moves and has no stats beyond its template level, HP and defences, so
// every other answer here is the neutral one a creature with no stat
// modifiers gives.
package doorhit

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// The base stats every door has: doors.xml sets none, so each is the
// creature template default.
const (
	baseSTR = 40
	baseCON = 21
	baseDEX = 30
	baseINT = 20
	baseWIT = 43
	baseMEN = 20
)

// Target is a door as the target of a hit.
type Target struct{ *door.Object }

var (
	_ attackable.Combatant  = Target{}
	_ creature.FormulaActor = Target{}
	_ dynamic.StandIn       = Target{}
)

// Of returns d as a hit target.
func Of(d *door.Object) Target { return Target{Object: d} }

// StoredObject is the door itself, the object geodata holds while it is
// closed: a sight query to the hit target leaves the door out.
func (t Target) StoredObject() dynamic.Object { return t.Object }

// CollisionRadius is the body radius every door has.
func (Target) CollisionRadius() float64 { return door.CollisionRadius }

func (Target) Karma() int             { return 0 }
func (t Target) AlikeDead() bool      { return t.Dead() }
func (Target) FakeDeath() bool        { return false }
func (Target) RecentFakeDeath() bool  { return false }
func (Target) IsMoving() bool         { return false }
func (Target) MovementDisabled() bool { return true }
func (Target) SilentMoving() bool     { return false }
func (Target) SpawnProtected() bool   { return false }
func (Target) CanGiveDamage() bool    { return true }
func (Target) RaidRelated() bool      { return false }
func (Target) SiegeGuard() bool       { return false }
func (Target) Guard() bool            { return false }

// Knows reports whether other is in the door's known list.
func (t Target) Knows(other attackable.Combatant) bool {
	tracked, ok := other.(world.Tracked)
	return ok && world.Knows(t.Object, tracked)
}

// TakeDamage lands an auto-attack hit on the door and reports whether it
// broke the door.
func (t Target) TakeDamage(damage int, attacker attackable.Combatant) bool {
	t.ReduceHP(float64(damage), attacker, modelskill.Definition{})
	return t.Dead()
}

func (Target) STR() int { return baseSTR }
func (Target) CON() int { return baseCON }
func (Target) DEX() int { return baseDEX }
func (Target) INT() int { return baseINT }
func (Target) WIT() int { return baseWIT }
func (Target) MEN() int { return baseMEN }

// PAtk and MAtk are zero: a door never attacks.
func (Target) PAtk() float64 { return 0 }
func (Target) MAtk() float64 { return 0 }

// PDef and MDef are the door's defences under the Seal of Strife.
func (t Target) PDef() float64 { return float64(t.PhysicalDefence()) }
func (t Target) MDef() float64 { return float64(t.MagicDefence()) }

func (Target) MagicCriticalRate() float64     { return 0 }
func (Target) AttackType() item.WeaponType    { return item.WeaponNone }
func (Target) SoulshotCharged() bool          { return false }
func (Target) SpiritshotCharged() bool        { return false }
func (Target) BlessedSpiritshotCharged() bool { return false }

// CalcStat returns base: a door carries no stat modifier.
func (Target) CalcStat(_ stat.Stat, base float64) float64 { return base }

func (Target) RandomDamageSpread() int  { return 0 }
func (Target) Roll(n int) int           { return rnd.Get(n) }
func (Target) WeaponGradePenalty() bool { return false }

// HP returns the door's current HP.
func (t Target) HP() float64 { return t.CurrentHP() }

// MaxHPValue returns the door's maximum HP.
func (t Target) MaxHPValue() float64 { return float64(t.MaxHP()) }

func (Target) Invul() bool { return false }

// ShieldDefense is ShieldFailed: a door carries no shield.
func (Target) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	return formulas.ShieldFailed
}

// RaceMultiplier is 1: a door has no race.
func (Target) RaceMultiplier(creature.FormulaActor) float64 { return 1 }

// Evasion is 0: a door has no evasion.
func (Target) Evasion() int        { return 0 }
func (Target) LethalRate() float64 { return 1 }

// PhysicalSkillInput resolves a physical skill's damage input against the
// door.
func (t Target) PhysicalSkillInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, t, def, false, t.RaceMultiplier(caster))
}

// MagicDamageInput resolves a magic skill's damage input against the door,
// rolling resist when magicFailures is set.
func (t Target) MagicDamageInput(caster creature.FormulaActor, def modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, t, def, false, magicFailures)
}

// BlowInput resolves a blow skill's damage input against the door.
func (t Target) BlowInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, t, def, false)
}

// SkillSuccessInput resolves a skill's landing roll against the door. The
// roll runs as on any creature, though nothing a door is hit with lands.
func (t Target) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return creature.ResolveSkillSuccessInput(caster, t, def, bss, shield)
}

// CounterSkillPhysical is 0: a door counters no skill.
func (Target) CounterSkillPhysical() float64 { return 0 }
