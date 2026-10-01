package cast

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// cubicMaxMagicRange is the 3D distance from its owner, collision radii
// left out, within which a cubic picks an enemy or a party member to heal.
const cubicMaxMagicRange = 900

// CubicFireOwner is the narrow owner surface a cubic fire attempt reads
// from: its currently selected target, its own RNG roll, its own vitals
// for the Life Cubic's self-heal gate, and itself as the attacker an enemy
// must be attackable by without a forced attack.
type CubicFireOwner interface {
	Target() world.Tracked
	Roll(n int) int
	CurrentHP() int
	MaxHPValue() float64
	Attacker() skilltarget.Actor
}

// cubicEnemy is a selected object a cubic may fire at when its owner could
// attack it without forcing.
type cubicEnemy interface {
	Target
	AttackableWithoutForceBy(caster skilltarget.Actor) bool
}

// CubicGrantedLevel resolves the level a granted cubic actually fires its
// skills at: skill 4338 (Life Cubic for Beginners) always grants level 8,
// and any granting-skill level above 100 (enchanted skill levels) collapses
// through a truncating integer-division formula before the cubic is added
// or refreshed.
func CubicGrantedLevel(def modelskill.Definition) int {
	switch {
	case int(def.ID) == 4338:
		return 8
	case def.Level > 100:
		// ((level - 100) / 7) + 8 in all-int arithmetic, truncating toward
		// zero; rounding the result is a no-op since it is already an
		// integer. Go's integer division truncates toward zero the same way.
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

// LifeCubicMember is one member of the Life Cubic owner's party as the
// heal-target scan reads it.
type LifeCubicMember struct {
	Target  Target
	Dead    bool
	HPRatio float64
}

// DecideLifeCubicTarget picks the Life Cubic's heal target. In a party
// (members non-nil, in party order, the owner among them) it is the living
// member under full HP with the lowest HP ratio within range of the owner,
// the first of equals; outside one it is the owner, when under full HP. A
// target is then healed only on a roll banded by its HP ratio.
func DecideLifeCubicTarget(owner CubicFireOwner, members []LifeCubicMember) (Target, bool) {
	var (
		target Target
		ratio  = 1.0
	)
	if members != nil {
		self, ok := owner.(Target)
		if !ok {
			return nil, false
		}
		ox, oy, oz := self.Position()
		for _, m := range members {
			if m.Dead || m.HPRatio >= 1.0 || ratio <= m.HPRatio {
				continue
			}
			mx, my, mz := m.Target.Position()
			if !location.In3DRadius(ox, oy, oz, mx, my, mz, cubicMaxMagicRange) {
				continue
			}
			target, ratio = m.Target, m.HPRatio
		}
	} else {
		self, ok := owner.(Target)
		maxHP := owner.MaxHPValue()
		if !ok || maxHP <= 0 {
			return nil, false
		}
		if hpRatio := float64(owner.CurrentHP()) / maxHP; hpRatio < 1.0 {
			target, ratio = self, hpRatio
		}
	}
	if target == nil {
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
	return target, true
}

// pickCubicEnemyTarget picks a cubic's enemy: the owner's selected object,
// within range of the owner, that the owner may attack without forcing.
// Whether a dead one is affected is left to the fired skill.
func pickCubicEnemyTarget(owner CubicFireOwner) (Target, bool) {
	selected, ok := owner.Target().(cubicEnemy)
	if !ok {
		return nil, false
	}
	self, ok := owner.(Target)
	if !ok {
		return nil, false
	}
	ox, oy, oz := self.Position()
	tx, ty, tz := selected.Position()
	if !location.In3DRadius(ox, oy, oz, tx, ty, tz, cubicMaxMagicRange) {
		return nil, false
	}
	if !selected.AttackableWithoutForceBy(owner.Attacker()) {
		return nil, false
	}
	return selected, true
}

// ApplyCubicHeal restores HP directly, matching Cubic.useHealSkill: a flat
// power * target's HEAL_EFFECTIVNESS / 100, with no caster stat or
// proficiency contribution — distinct from the generic HEAL skill handler a
// player's own heal cast goes through, which scales by the caster's own
// MATK and healing proficiency. A player target whose HP actually rose is
// sent its own status at once, as every HP restore does; a restore that
// applied nothing sends none, and a summon or NPC target already republished
// its status from AddHP. healed reports whether the target could be healed
// at all, so the caller knows whether to send the heal feedback message
// after that status.
func ApplyCubicHeal(power float32, target Target) (healed bool) {
	// Only an effect participant has HP to restore.
	healable, ok := target.(effect.Actor)
	if !ok || !healable.CanBeHealed() {
		return false
	}
	applied := healable.AddHP(float64(power) * healable.HealEffectiveness() / 100)
	if applied > 0 && healable.Kind() == actor.KindPlayer {
		healable.BroadcastStatus()
	}
	return true
}

// ApplyCubicEffect dispatches a non-Life cubic's fired skill directly to
// its pre-resolved single target, bypassing the normal target-type
// resolution phase ApplyEffectsResult runs (the cubic already picked its
// own target via DecideCubicFire), as a cubic's default skill-handler
// dispatch does.
// A cubic's own target selection resolves world objects rather than the
// cast-participant surface a skill handler acts on, so a target that isn't
// actor-shaped is dropped here instead of reaching the handlers as a value
// none of their assertions can match.
// The returned EffectResult carries AttackFailed (and any other handler
// outcome) back to the caller: a failed offensive continuous roll must
// still reach the owner as ATTACK_FAILED, not be dropped silently.
// The cast is marked Cubic: the continuous and disabler cubic branches
// read the owner's blessed-spiritshot charge but never spend it.
// sink, when non-nil, delivers each handler message as it is produced, in
// place of EffectResult.Messages.
func ApplyCubicEffect(skills *handlerskill.Registry, caster handlerskill.Creature, def modelskill.Definition, target Target, sink handlerskill.MessageSink) EffectResult {
	if skills == nil {
		return EffectResult{}
	}
	actor, ok := target.(handlerskill.Actor)
	if !ok {
		return EffectResult{}
	}
	result, ok := skills.UseResult(handlerskill.Cast{Caster: caster, Skill: def, Targets: []handlerskill.Actor{actor}, Cubic: true, Sink: sink})
	if !ok {
		return EffectResult{}
	}
	return EffectResult{
		Handled:           true,
		Messages:          result.Messages,
		AttackFailed:      result.AttackFailed,
		Counterattacks:    result.Counterattacks,
		Lethals:           result.Lethals,
		Dodges:            result.Dodges,
		Resisted:          result.Resisted,
		MagicResists:      result.MagicResists,
		ManaDamageMissed:  result.ManaDamageMissed,
		ManaDrains:        result.ManaDrains,
		OpponentMPReduced: result.OpponentMPReduced,
		CubicAdded:        result.CubicAdded,
		CubicTargets:      result.CubicTargets,
		CubicAddedTargets: result.CubicAddedTargets,
		CubicTouched:      result.CubicTouched,
		CubicID:           result.CubicID,
	}
}
