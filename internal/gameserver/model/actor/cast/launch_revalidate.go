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

// RevalidateLaunch runs the oracle's launch-phase mid-cast recheck
// (CreatureCast.onMagicLaunch: target-lost, escape range, line of sight,
// peace zone), skipped entirely when target is the caster itself. caster
// and target only need to satisfy the optional surfaces each gate uses
// (Knows, CollisionRadius, SightChecker, EffectRangeInPeaceZone,
// skilltarget.Creature); a caster or target that doesn't implement one
// leaves that gate permissive, matching the graceful-degradation pattern
// this port already uses for actor state not modeled yet.
func RevalidateLaunch(caster, target Target, def modelskill.Definition) LaunchAbortReason {
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

	if def.Offensive && isLaunchPlayable(caster) && isLaunchPlayable(target) {
		if inOwnPeaceZone(caster) {
			return LaunchAbortCasterPeaceZone
		}
		if inOwnPeaceZone(target) {
			return LaunchAbortTargetPeaceZone
		}
	}

	return LaunchAbortNone
}

func sameLaunchTarget(a, b Target) bool {
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

	total := float64(rangeVal) + launchCollisionRadius(a) + launchCollisionRadius(b)
	return float64(distSq) <= total*total
}

func launchCollisionRadius(t Target) float64 {
	if cr, ok := t.(interface{ CollisionRadius() float64 }); ok {
		return cr.CollisionRadius()
	}
	return 0
}

func launchCanSee(caster, target Target) bool {
	creature, ok := target.(skilltarget.Creature)
	if !ok {
		return true
	}
	checker, ok := caster.(skilltarget.SightChecker)
	if !ok {
		return true
	}
	return checker.CanSeeTarget(creature)
}

func launchTargetLost(caster, target Target, skillType string) bool {
	if skillType == "SUMMON_FRIEND" {
		return false
	}
	knower, ok := caster.(interface {
		Knows(attackable.Combatant) bool
	})
	if !ok {
		return false
	}
	combatant, ok := target.(attackable.Combatant)
	if !ok {
		return false
	}
	return !knower.Knows(combatant)
}

func isLaunchPlayable(t Target) bool {
	creature, ok := t.(skilltarget.Creature)
	return ok && creature.Category().Has(skilltarget.CategoryPlayable)
}

func inOwnPeaceZone(t Target) bool {
	zoned, ok := t.(interface {
		EffectRangeInPeaceZone(x, y, z, effectRange int) bool
	})
	if !ok {
		return false
	}
	x, y, z := t.Position()
	return zoned.EffectRangeInPeaceZone(x, y, z, 0)
}
