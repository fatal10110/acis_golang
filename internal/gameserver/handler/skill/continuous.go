package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// fearImmunePlayableSkillIDs are the skills whose FEAR effect must not land on
// a player-controlled target even when the success roll would otherwise pass.
// The reference encodes this as a constant list on the fear effect class; it
// is reproduced here so the caster-side gating the continuous handler performs
// matches it.
var fearImmunePlayableSkillIDs = map[modelskill.ID]bool{98: true, 1272: true, 1381: true}

// continuousTarget is the surface a continuous (buff/debuff/over-time) skill
// acts on: an alive actor carrying a live effect list.
type continuousTarget interface {
	effectListTarget
	Dead() bool
}

// invulnerableCaster is implemented by casters that can report invulnerability,
// which blocks the heal-over-time family from ticking while the caster is
// immune.
type invulnerableCaster interface {
	Invul() bool
}

// playableTarget reports whether the target is player-controlled, the gate the
// FEAR skill-id exception list keys on.
type playableTarget interface {
	Playable() bool
}

type continuousHandler struct{}

// Types lists the 14 skill types the reference continuous handler covers.
func (continuousHandler) Types() []string {
	return []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF", "FUSION",
	}
}

func (h continuousHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h continuousHandler) UseResult(cast Cast) Result {
	var result Result
	skillType := skillTypeKey(cast.Skill.SkillType)

	for _, obj := range cast.Targets {
		target, ok := obj.(continuousTarget)
		if !ok || target.Dead() {
			continue
		}

		effected := h.reflectTarget(cast, target)
		if effected == nil {
			continue
		}

		switch skillType {
		case "BUFF":
			if hasEffectType(effected.EffectList(), "BLOCK_BUFF") {
				continue
			}
			// A cursed-weapon holder can neither receive nor bestow buffs.
			// The reference exempts clan-hall manager NPCs and resolves the
			// caster's cursed state through its acting player; neither marker
			// exists on this layer yet, so this only gates on the caster and
			// target directly and skips the exception.
			if !sameObject(cast.Caster, effected) && (cursed(effected) || cursed(cast.Caster)) {
				continue
			}
		case "HOT", "MPHOT":
			if c, ok := cast.Caster.(invulnerableCaster); ok && c.Invul() {
				continue
			}
		case "FEAR":
			if pt, ok := effected.(playableTarget); ok && pt.Playable() && fearImmunePlayableSkillIDs[cast.Skill.ID] {
				continue
			}
		}

		// Target under debuff immunity.
		if cast.Skill.Offensive && hasEffectType(effected.EffectList(), "BLOCK_DEBUFF") {
			continue
		}

		// Offensive and debuff skills roll to land, folding in the caster's
		// blessed-spiritshot charge and the target's shield-block outcome
		// against this cast; everything else acts unconditionally.
		acted := true
		if cast.Skill.Offensive || cast.Skill.Debuff {
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
			acted = ok && succeeded
		}

		if !acted {
			result.AttackFailed++
			continue
		}

		// A toggle refresh drops the prior same-skill effect before reapplying.
		if cast.Skill.Activation == modelskill.ActivationToggle {
			stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
		}

		applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)

		if skillType == "AGGDEBUFF" {
			fireAggressionEvent(cast.Caster, effected, cast.Skill)
		}
	}

	applySelfEffects(cast.Caster, cast.Skill)
	return result
}

// reflectTarget returns the actual effect destination: the original target, or
// the caster when the target reflects the skill back. It returns nil when the
// skill reflects but the caster isn't itself a valid continuous target, which
// is safer dropped than guessed through.
func (continuousHandler) reflectTarget(cast Cast, target continuousTarget) continuousTarget {
	src, ok := target.(skillReflectSource)
	if !ok {
		return target
	}
	if !formulas.SkillReflects(src.SkillReflectInput(cast.Skill), rnd.Get(100)) {
		return target
	}
	self, ok := cast.Caster.(continuousTarget)
	if !ok {
		return nil
	}
	return self
}

// aggressionNotifiable is implemented by an attackable target that can react
// to an incoming AI aggression notification carrying the landed skill's
// power; a target without one doesn't react to it yet.
type aggressionNotifiable interface {
	NotifyAggression(source any, power int)
}

// retargetableOnAggression is implemented by a playable target that tracks
// a currently selected target and can be provoked into attacking the source
// of a landed aggression-debuff effect; a target without one isn't
// retargeted yet.
type retargetableOnAggression interface {
	CurrentTarget() any
	SetTarget(any)
	AttackTarget(any)
}

// fireAggressionEvent runs the post-landing aggression notification an
// AGGDEBUFF-type effect triggers: an attackable target is notified of the
// caster's aggression at the skill's power, while a playable target is
// provoked into attacking the caster if it was already targeting it, or
// retargeted onto the caster otherwise. A target implementing neither
// optional surface is left as-is.
func fireAggressionEvent(caster, effected any, def modelskill.Definition) {
	if am, ok := effected.(attackableMarker); ok && am.Attackable() {
		if n, ok := effected.(aggressionNotifiable); ok {
			n.NotifyAggression(caster, int(def.Power))
		}
		return
	}
	if pt, ok := effected.(playableTarget); ok && pt.Playable() {
		r, ok := effected.(retargetableOnAggression)
		if !ok {
			return
		}
		if sameObject(r.CurrentTarget(), caster) {
			r.AttackTarget(caster)
		} else {
			r.SetTarget(caster)
		}
	}
}
