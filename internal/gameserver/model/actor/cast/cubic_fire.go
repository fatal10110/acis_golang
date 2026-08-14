package cast

import (
	"math"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// cubicMaxMagicRange is Cubic.MAX_MAGIC_RANGE: the 3D range (collision radii
// included) both pickCubicEnemyTarget and DecideLifeCubicTarget scan within.
const cubicMaxMagicRange = 900

// CubicFireOwner is the narrow owner surface a cubic fire attempt reads
// from: its currently selected target, its own RNG roll, and its own vitals
// for the Life Cubic's self-heal gate.
type CubicFireOwner interface {
	Target() any
	Roll(n int) int
	CurrentHP() int
	MaxHPValue() float64
}

// CubicGrantedLevel resolves the level a granted cubic actually fires its
// skills at, matching L2SkillSummon.useSkill's skillLevel adjustment: skill
// 4338 (Life Cubic for Beginners) always grants level 8, and any
// granting-skill level above 100 (enchanted skill levels) collapses through
// the reference's own truncating-integer-division formula before ever
// reaching CubicList.addOrRefreshCubic.
func CubicGrantedLevel(def modelskill.Definition) int {
	switch {
	case int(def.ID) == 4338:
		return 8
	case def.Level > 100:
		// Java: Math.round(((getLevel() - 100) / 7) + 8) — all-int
		// arithmetic, truncating toward zero; the Math.round call is a
		// no-op since the operand is already an integer by then. Go's
		// integer division truncates toward zero the same way.
		return (def.Level-100)/7 + 8
	default:
		return def.Level
	}
}

// DecideCubicFire resolves a non-Life cubic's action-tick activation-chance
// roll, random skill pick among skillIDs, and enemy target selection,
// matching Cubic.fireAction's non-Life branch. ok reports whether the cubic
// should fire at all this tick — a failed activation roll or no valid
// target both report false with no other observable effect.
func DecideCubicFire(owner CubicFireOwner, skillIDs []int, activationChance int) (skillID int, target Target, ok bool) {
	if owner == nil || len(skillIDs) == 0 {
		return 0, nil, false
	}
	if owner.Roll(100) >= activationChance {
		return 0, nil, false
	}
	skillID = skillIDs[owner.Roll(len(skillIDs))]
	target, ok = pickCubicEnemyTarget(owner)
	if !ok {
		return 0, nil, false
	}
	return skillID, target, true
}

// DecideLifeCubicTarget mirrors Cubic.pickFriendlyTarget's no-party
// fallback: heal the owner if under full HP, gated by the reference's
// HP-ratio-banded probability roll. The party-scan branch needs the
// milestone-M8 party system and isn't reachable yet.
func DecideLifeCubicTarget(owner CubicFireOwner) (Target, bool) {
	self, ok := owner.(Target)
	if !ok {
		return nil, false
	}
	maxHP := owner.MaxHPValue()
	if maxHP <= 0 {
		return nil, false
	}
	ratio := float64(owner.CurrentHP()) / maxHP
	if ratio >= 1.0 {
		return nil, false
	}

	roll := owner.Roll(100)
	var chance int
	switch {
	case ratio > 0.6:
		chance = 13
	case ratio < 0.3:
		chance = 53
	default:
		chance = 33
	}
	if roll > chance {
		return nil, false
	}
	return self, true
}

// pickCubicEnemyTarget mirrors Cubic.pickEnemyTarget: the owner's currently
// selected target, if within range and not already dead. The reference's
// full isAttackableWithoutForceBy alignment/karma matrix is not replicated
// here — deferred, see fatal10110/acis_golang#1129.
func pickCubicEnemyTarget(owner CubicFireOwner) (Target, bool) {
	selected := owner.Target()
	if selected == nil {
		return nil, false
	}
	combatant, ok := selected.(attackable.Combatant)
	if !ok || combatant.AlikeDead() {
		return nil, false
	}
	target, ok := selected.(Target)
	if !ok {
		return nil, false
	}
	ownerTarget, ok := owner.(Target)
	if !ok || !cubicWithinRange(ownerTarget, target) {
		return nil, false
	}
	return target, true
}

func cubicWithinRange(a, b Target) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	dx := float64(ax - bx)
	dy := float64(ay - by)
	dz := float64(az - bz)
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	total := float64(cubicMaxMagicRange) + cubicCollisionRadius(a) + cubicCollisionRadius(b)
	return dist <= total
}

func cubicCollisionRadius(t Target) float64 {
	if cr, ok := t.(interface{ CollisionRadius() float64 }); ok {
		return cr.CollisionRadius()
	}
	return 0
}

// cubicHealTarget and cubicHealEffectiveness are the narrow surfaces
// ApplyCubicHeal needs from a Life Cubic's heal target.
type cubicHealTarget interface {
	CanBeHealed() bool
	AddHP(float64) float64
}

type cubicHealEffectiveness interface {
	HealEffectiveness() float64
}

// ApplyCubicHeal restores HP directly, matching Cubic.useHealSkill: a flat
// power * target's HEAL_EFFECTIVNESS / 100, with no caster stat or
// proficiency contribution — distinct from the generic HEAL skill handler a
// player's own heal cast goes through, which scales by the caster's own
// MATK and healing proficiency. healed reports whether the target actually
// received HP, so the caller knows whether to send the heal feedback packet.
func ApplyCubicHeal(power float32, target any) (healed bool) {
	healable, ok := target.(cubicHealTarget)
	if !ok || !healable.CanBeHealed() {
		return false
	}
	effectiveness := 100.0
	if eff, ok := target.(cubicHealEffectiveness); ok {
		effectiveness = eff.HealEffectiveness()
	}
	healable.AddHP(float64(power) * effectiveness / 100)
	return true
}

// ApplyCubicEffect dispatches a non-Life cubic's fired skill directly to
// its pre-resolved single target, bypassing the normal target-type
// resolution phase ApplyEffectsResult runs (the cubic already picked its
// own target via DecideCubicFire), matching Cubic.fireAction's default
// SkillHandler dispatch.
// A cubic's own target selection resolves world objects rather than the
// cast-participant surface a skill handler acts on, so a target that isn't
// actor-shaped is dropped here instead of reaching the handlers as a value
// none of their assertions can match.
func ApplyCubicEffect(skills *handlerskill.Registry, caster handlerskill.Actor, def modelskill.Definition, target Target) {
	if skills == nil {
		return
	}
	actor, ok := target.(handlerskill.Actor)
	if !ok {
		return
	}
	skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: []handlerskill.Actor{actor}})
}
