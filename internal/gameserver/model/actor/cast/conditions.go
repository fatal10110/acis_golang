package cast

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// ConditionError is a cast refused by one of the skill's <cond> clauses.
// Clause carries the feedback that clause configures.
type ConditionError struct {
	Clause modelskill.ConditionClause
}

func (*ConditionError) Error() string { return "cast: skill condition not met" }

// conditionGate is the optional Actor capability CanCast asks before paying
// item costs: whether def's <cond> clauses hold against target. Player and
// summon casters evaluate them; NPC casts never do.
type conditionGate interface {
	SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool)
}

// olympiadGate is the optional Actor capability CanCast asks after the
// <cond> clauses: whether the player acting for the caster (the player
// itself, or a summon's owner) is in Olympiad mode, where hero and
// RESURRECT skills are refused. NPC casts never ask.
type olympiadGate interface {
	ActingPlayerInOlympiad() bool
}

// servitorGate is the optional Actor capability CanCast asks before a
// servitor summon pays anything: whether the player already has a summon
// out, rides a mount, or is mid-swing. Only a player caster answers it.
type servitorGate interface {
	HasSummon() bool
	Mounted() bool
	AttackingNow() bool
}

// boatRider is the optional Actor capability CanCast asks after the
// servitorGate checks: whether the player rides a boat.
type boatRider interface {
	InBoat() bool
}

// ServitorSummon reports whether def summons a servitor rather than a cubic.
func ServitorSummon(def modelskill.Definition) bool {
	return def.SkillType == "SUMMON" && !def.IsCubic
}

// HasSummon reports whether the player has a summon in the world. A pet
// still resolving its pets row is not in it yet.
func (a PlayerActor) HasSummon() bool {
	if a.Character == nil {
		return false
	}
	_, ok := a.Character.Summon()
	return ok
}

// Mounted reports whether the player rides a mount.
func (a PlayerActor) Mounted() bool {
	return a.Character != nil && a.Character.Mounted()
}

// InBoat reports whether the player rides a boat.
func (a PlayerActor) InBoat() bool {
	return a.Character != nil && a.Character.InBoat()
}

// AttackingNow reports whether the player's swing is still in flight. The
// attack stance that outlives a swing does not count.
func (a PlayerActor) AttackingNow() bool {
	return a.Attack != nil && a.Attack.AttackingNow()
}

// olympiadRestricted reports whether def is barred from Olympiad matches.
func olympiadRestricted(def modelskill.Definition) bool {
	return def.HeroSkill || def.SkillType == "RESURRECT"
}

// ActingPlayerInOlympiad reports whether the player is in Olympiad mode.
func (a PlayerActor) ActingPlayerInOlympiad() bool {
	return a.Character != nil && a.Character.OlympiadMode()
}

// ActingPlayerInOlympiad reports whether the summon's owner is in Olympiad
// mode; an ownerless summon never is.
func (a SummonActor) ActingPlayerInOlympiad() bool {
	if a.Summon == nil {
		return false
	}
	owner, ok := a.Summon.Owner()
	if !ok {
		return false
	}
	player, ok := owner.(interface{ OlympiadMode() bool })
	return ok && player.OlympiadMode()
}

// SkillConditions evaluates def's <cond> clauses with the player as caster.
func (a PlayerActor) SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool) {
	return conditions.EvaluateSkill(def, a.Character, target)
}

// SkillConditions evaluates def's <cond> clauses with the summon as caster.
func (a SummonActor) SkillConditions(target Target, def modelskill.Definition) (modelskill.ConditionClause, bool) {
	return conditions.EvaluateSkill(def, a.Summon, target)
}

// WeaponAllowed reports whether a creature holding the item types in held
// (see Actor.HeldItemTypeMask) may use def: def names no weapon or shield
// type, or held shares a bit with one it names. A name that is no weapon
// or armor type restricts nothing.
func WeaponAllowed(def modelskill.Definition, held int32) bool {
	if def.WeaponsAllowed == "" {
		return true
	}
	allowed := item.ParseWornKindMask(def.WeaponsAllowed)
	return allowed == 0 || held&allowed != 0
}
