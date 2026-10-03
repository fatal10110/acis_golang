package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// AttackDisabled reports whether this summon cannot start a physical attack:
// dead, paralyzed, or held by stun, sleep, fear, or an immobile-until-attacked
// effect. Unlike DenyAIAction, teleporting does not disable attacking; it
// gates movement instead.
// See effectHeld for the effects it counts while AbortAll runs.
func (a *Actor) AttackDisabled() bool {
	return a.AlikeDead() || a.paralyzedLock() || a.effectHeld(effect.AIDenyFlags)
}

// MovementDisabled reports whether this summon cannot move. Fear is not
// included: it is an out-of-control state, not a movement lock. Sit/stand
// do not apply to summons.
func (a *Actor) MovementDisabled() bool {
	if a.AlikeDead() || a.Paralyzed() || a.Teleporting() || a.Immobilized() {
		return true
	}
	if a.effects == nil {
		return false
	}
	return a.effects.IsAffected(effect.FlagStunned | effect.FlagMeditating | effect.FlagSleep | effect.FlagRooted)
}

func (a *Actor) IsMoving() bool { return a.Move().Moving() }

// MovingTo returns the target of a's leg in flight and whether it is walking
// at all.
func (a *Actor) MovingTo() (location.Location, bool) { return a.Move().MovingTo() }

func (a *Actor) InAttackRange(target attackable.Combatant) bool {
	x, y, z := a.Position()
	return attack.InPhysicalRange(location.Location{X: x, Y: y, Z: z}, a.PhysicalAttackRange(), a.CollisionRadius(), target)
}

func (a *Actor) ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant)) {
	if a.world == nil {
		return
	}
	a.world.ForEachKnownInRadius(a, radius, func(candidate world.Tracked) {
		if combatant, ok := candidate.(attackable.Combatant); ok {
			fn(combatant)
		}
	})
}

func (a *Actor) PoleAttackAngle() int {
	return int(a.calcStat(stat.PowerAttackAngle, 120))
}

func (a *Actor) PoleAttackCountMax() int {
	for _, active := range a.EffectList().All() {
		if active.Type == effect.TypePolearmTargetSingle {
			return 1
		}
	}
	return int(a.calcStat(stat.AttackCountMax, 0))
}

// LineOfSight reports whether two actors have a geodata-obstructed view.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// lineOfSightIgnoring is the optional LineOfSight query that leaves one
// dynamic geodata object out, used when the target is such an object.
type lineOfSightIgnoring interface {
	CanSeeActorIgnoring(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64, ignore dynamic.Object) bool
}

// The production geo must satisfy the optional query: canSeeObject finds it
// by type assertion, so a signature drift would otherwise silently make a
// target door hide itself again.
var _ lineOfSightIgnoring = move.EngineGeo{}

// CanSee reports whether target is visible through geodata, or permits the
// check when no query is attached (such as isolated domain tests).
func (a *Actor) CanSee(target attackable.Combatant) bool {
	tx, ty, tz := target.Position()
	return a.canSeeObject(target, tx, ty, tz, target.CollisionHeight())
}

// canSeeObject queries sight to obj standing at (tx, ty, tz). An obj that
// is itself a geodata object (a closed door) is left out of the query, so it
// never hides itself.
func (a *Actor) canSeeObject(obj any, tx, ty, tz int, theight float64) bool {
	if a.los == nil {
		return true
	}
	ox, oy, oz := a.Position()
	if geoObj, ok := obj.(dynamic.Object); ok {
		if los, ok := a.los.(lineOfSightIgnoring); ok {
			return los.CanSeeActorIgnoring(ox, oy, oz, a.CollisionHeight(), tx, ty, tz, theight, geoObj)
		}
	}
	return a.los.CanSeeActor(ox, oy, oz, a.CollisionHeight(), tx, ty, tz, theight)
}
func (a *Actor) AttackSpeed() int                { return int(a.PhysicalAttackSpeed()) }
func (a *Actor) WeaponReuseDelay() time.Duration { return 0 }
func (a *Actor) ConsumeBowMP()                   {}
func (a *Actor) WeaponGrade() int                { return 0 }

// InPeaceZone reports whether this summon's zones hold it in peace.
func (a *Actor) InPeaceZone() bool { return a.membership.has(zone.FlagPeace) }

func (a *Actor) Evasion() int { return int(a.EvasionRate()) }

func (a *Actor) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}
	// Folding this needs the world known-list to hand out formula operands.
	// CreatureActor.ForEachKnownCombatantInRadius (attack/controller.go:47)
	// yields attackable.Combatant, narrowed from world.Tracked, and world
	// cannot name creature.FormulaActor because creature already depends on
	// world. The pole and split paths draw their extra targets from that
	// callback, so the narrowing stays here, at the one point where a
	// combatant becomes a formula operand; see #2362.
	other, ok := target.(creature.FormulaActor)
	if !ok {
		hit.Miss = true
		return hit
	}
	_, _, z := a.Position()
	_, _, targetZ := other.Position()
	behind, inFront := creature.AttackFacing(other, a)
	if formulas.Missed(formulas.HitRate(int(a.Accuracy()), other.Evasion(), z-targetZ, a.EffectList().IsNight(), behind, inFront), a.Roll(1000)) {
		hit.Miss = true
		return hit
	}
	crit := formulas.CritSucceeds(a.CriticalRate(a.combatStats().CritRate), a.Roll(1000))
	in, shield := creature.ResolvePhysicalAttackInput(a, other, crit)
	hit.Damage = creature.ApplyPhysicalAttackDamage(in, shield, split)
	hit.Crit = crit
	hit.Shield = shield
	return hit
}

func (a *Actor) BroadcastAttack(snapshot event.Attack) {
	a.emit(snapshot)
}
