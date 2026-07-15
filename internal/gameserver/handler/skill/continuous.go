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
// AGGDEBUFF's aggression AI event is dropped after the effect lands (see Use)
// because the AI-aggression pipeline isn't wired into this layer yet.
func (continuousHandler) Types() []string {
	return []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF", "FUSION",
	}
}

func (h continuousHandler) Use(cast Cast) {
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

		// Offensive and debuff skills roll to land; everything else acts
		// unconditionally. The reference also passes a shield-block outcome
		// and blessed-spiritshot charge into the roll; neither is wired
		// through this layer's landing-rate source yet, so the roll matches
		// the reduced-input form the other effect-landing handlers share.
		acted := true
		if cast.Skill.Offensive || cast.Skill.Debuff {
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, cast.Skill)
			acted = ok && succeeded
		}

		if !acted {
			// The reference sends ATTACK_FAILED to the caster here. Network
			// sends are not this handler's concern; the cast pipeline owns them.
			continue
		}

		// A toggle refresh drops the prior same-skill effect before reapplying.
		if cast.Skill.Activation == modelskill.ActivationToggle {
			stopEffectsBySkillID(effected.EffectList(), cast.Skill.ID)
		}

		applyEffects(cast.Caster, effected, cast.Skill, cast.Skill.Effects)

		if skillType == "AGGDEBUFF" {
			// The reference then fires an aggression AI event on an attackable
			// target, or retargets/attacks a playable. That AI-aggression
			// pipeline isn't wired into this handler layer yet, so only the
			// debuff effect itself lands here.
		}
	}

	applySelfEffects(cast.Caster, cast.Skill)
}

// reflectTarget returns the actual effect destination: the original target, or
// the caster when the target reflects the skill back. It returns nil when the
// skill reflects but the caster isn't itself a valid continuous target, which
// is safer dropped than guessed through.
func (continuousHandler) reflectTarget(cast Cast, target continuousTarget) continuousTarget {
	src, ok := any(target).(skillReflectSource)
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
