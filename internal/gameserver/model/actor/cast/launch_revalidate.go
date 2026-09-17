package cast

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// LaunchAbortReason identifies which of the launch-phase mid-cast
// revalidation gates stopped a cast, so a caller can map it to the
// reference's distinct system messages (or none, for a lost target).
type LaunchAbortReason int

const (
	// LaunchAbortNone means every gate passed; the cast continues.
	LaunchAbortNone LaunchAbortReason = iota
	// LaunchAbortTargetLost means the target is no longer known to the
	// caster. No system message accompanies this in the reference.
	LaunchAbortTargetLost
	// LaunchAbortTooFar means the target left the skill's escape range.
	LaunchAbortTooFar
	// LaunchAbortNoLineOfSight means the caster can no longer see the
	// target.
	LaunchAbortNoLineOfSight
	// LaunchAbortCasterPeaceZone means an offensive playable-vs-playable
	// cast's caster entered a peace zone mid-cast.
	LaunchAbortCasterPeaceZone
	// LaunchAbortTargetPeaceZone means an offensive playable-vs-playable
	// cast's target entered a peace zone mid-cast.
	LaunchAbortTargetPeaceZone
)

// LaunchCaster is the creature whose skill launch is revalidated.
type LaunchCaster interface {
	attackable.Combatant
	skilltarget.Actor
}

// RevalidateLaunch runs the oracle's launch-phase mid-cast recheck
// (CreatureCast.onMagicLaunch: target-lost, escape range, line of sight,
// peace zone), skipped entirely when target is the caster itself. A target
// that is not a creature (a door) is never lost, has no collision radius and
// no peace zone of its own.
func RevalidateLaunch(caster LaunchCaster, target Target, def modelskill.Definition) LaunchAbortReason {
	if caster == nil || target == nil || sameLaunchTarget(caster, target) {
		return LaunchAbortNone
	}

	if launchTargetLost(caster, target, def.SkillType) {
		return LaunchAbortTargetLost
	}

	if escapeRange := launchEscapeRange(def); escapeRange > 0 && !withinLaunchRange(escapeRange, caster, target) {
		return LaunchAbortTooFar
	}

	if def.Radius > 0 && !launchCanSee(caster, target) {
		return LaunchAbortNoLineOfSight
	}

	if creature, ok := target.(attackable.Combatant); ok && def.Offensive && caster.Kind().Playable() && creature.Kind().Playable() {
		if inOwnPeaceZone(caster) {
			return LaunchAbortCasterPeaceZone
		}
		if inOwnPeaceZone(creature) {
			return LaunchAbortTargetPeaceZone
		}
	}

	return LaunchAbortNone
}

// FusionChannelValid reports whether a live fusion channel still has range
// and line of sight to its target. Fusion does not run normal launch gates.
func FusionChannelValid(caster LaunchCaster, target Target, castRange int) bool {
	return caster != nil && target != nil && withinLaunchRange(castRange, caster, target) && launchCanSee(caster, target)
}

func sameLaunchTarget(a LaunchCaster, b Target) bool {
	return a.ObjectID() == b.ObjectID()
}

// launchEscapeRange mirrors CreatureCast.onMagicLaunch's escape-range
// derivation: the skill's effect range if set, else its radius when the
// skill has no cast range and a radius bigger than the default, else no
// range check at all.
func launchEscapeRange(def modelskill.Definition) int {
	if def.EffectRange > 0 {
		return def.EffectRange
	}
	if def.CastRange <= 0 && def.Radius > 80 {
		return def.Radius
	}
	return 0
}

// withinLaunchRange reports whether a and b are within rangeVal of each
// other in 3D, including both actors' collision radii, matching
// MathUtil.checkIfInRange(range, actor, target, true).
func withinLaunchRange(rangeVal int, a, b Target) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	dx := int64(ax - bx)
	dy := int64(ay - by)
	dz := int64(az - bz)
	distSq := dx*dx + dy*dy + dz*dz

	total := float64(rangeVal) + collisionRadius(a) + collisionRadius(b)
	return float64(distSq) <= total*total
}

// collisionRadius is t's body radius, or 0 for a target that is not a
// creature.
func collisionRadius(t Target) float64 {
	if c, ok := t.(attackable.Combatant); ok {
		return c.CollisionRadius()
	}
	return 0
}

func launchCanSee(caster LaunchCaster, target Target) bool {
	creature, ok := target.(skilltarget.Actor)
	return !ok || caster.CanSeeTarget(creature)
}

func launchTargetLost(caster LaunchCaster, target Target, skillType string) bool {
	if skillType == "SUMMON_FRIEND" {
		return false
	}
	combatant, ok := target.(attackable.Combatant)
	return ok && !caster.Knows(combatant)
}

func inOwnPeaceZone(c attackable.Combatant) bool {
	x, y, z := c.Position()
	return c.EffectRangeInPeaceZone(x, y, z, 0)
}
