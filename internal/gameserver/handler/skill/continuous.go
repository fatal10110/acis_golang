package skill

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fearImmunePlayableSkillIDs are the skills whose FEAR effect must not land on
// a player-controlled target even when the success roll would otherwise pass.
// The reference encodes this as a constant list on the fear effect class; it
// is reproduced here so the caster-side gating the continuous handler performs
// matches it.
var fearImmunePlayableSkillIDs = map[modelskill.ID]bool{98: true, 1272: true, 1381: true}

type continuousHandler struct {
	defs Definitions
}

// Types lists 13 of the 14 skill types the reference continuous handler's
// SKILL_IDS covers (Continuous.java:28-44); FUSION is the 14th but is
// omitted here. For a player caster it's a dead entry in Java too — PlayerAI
// routes FUSION to PlayerCast.doFusionCast before callSkill ever runs
// (PlayerAI.java:300-301), applying it via startFusionSkill at cast start
// instead (PlayerCast.java:81-84), matched here by fusionHandler. A
// non-player caster has no such diversion — CreatureCast.doFusionCast is an
// uncalled stub (CreatureCast.java:81-84) — so its hit-time callSkill()
// dispatches FUSION straight into Continuous.SKILL_IDS' entry, same as any
// other skill type (CreatureCast.java:493-497). It applies the caster
// skill's own effects, which every datapack FUSION skill (e.g. 3626-3628,
// skills/3600-3699.xml) carries none of, so the observable result is a
// no-op rather than the entry never firing at all.
func (continuousHandler) Types() []string {
	return []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF",
	}
}

func (h continuousHandler) Use(cast Cast) {
	h.UseResult(cast)
}

func (h continuousHandler) UseResult(cast Cast) Result {
	var result Result
	def := h.effectSkill(cast.Skill)
	skillType := skillTypeKey(def.SkillType)

	for _, obj := range cast.Targets {
		// The dead-target skip diverges from the reference handler, which
		// has no such gate and applies effects to a target that died
		// between the cast's launch and its hit. Deferred, tracked so it
		// stays linked rather than silently approximated (issue #2384).
		target, ok := asCreature(obj)
		if !ok || target.Dead() {
			continue
		}

		effected := h.reflectTarget(cast.Caster, def, target)
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
			if cast.Caster != nil && cast.Caster.Invul() {
				continue
			}
		case "FEAR":
			if effected.Kind().Playable() && fearImmunePlayableSkillIDs[def.ID] {
				continue
			}
		}

		// Target under debuff immunity.
		if def.Offensive && hasEffectType(effected.EffectList(), "BLOCK_DEBUFF") {
			continue
		}

		// Offensive and debuff skills roll to land, folding in the caster's
		// blessed-spiritshot charge and the target's shield-block outcome
		// against this cast; everything else acts unconditionally.
		acted := true
		if def.Offensive || def.Debuff {
			succeeded, ok := checkSkillSuccess(cast.Caster, effected, def)
			acted = ok && succeeded
		}

		if !acted {
			result.AttackFailed++
			continue
		}

		// A toggle refresh drops the prior same-skill effect before reapplying.
		if def.Activation == modelskill.ActivationToggle {
			stopEffectsBySkillID(effected.EffectList(), def.ID)
		}

		applyCastEffects(cast, effected, def, def.Effects)

		if skillType == "AGGDEBUFF" {
			fireAggressionEvent(cast.Caster, effected, def)
		}
	}

	applySelfEffects(cast, def)
	return result
}

func (h continuousHandler) effectSkill(def modelskill.Definition) modelskill.Definition {
	if def.EffectID == 0 || h.defs == nil {
		return def
	}
	level := def.EffectLevel
	if level == 0 {
		level = 1
	}
	if resolved, ok := h.defs.Definition(modelskill.Ref{ID: modelskill.ID(def.EffectID), Level: level}); ok {
		return resolved
	}
	return def
}

// reflectTarget returns the actual effect destination: the original target, or
// the caster when the target reflects the skill back.
func (continuousHandler) reflectTarget(caster Creature, def modelskill.Definition, target Creature) Creature {
	in := target.SkillReflectInput(def)
	in.SkillType = skillTypeKey(def.SkillType)
	if !formulas.SkillReflects(in, rnd.Get(100)) {
		return target
	}
	return caster
}

// fireAggressionEvent runs the post-landing aggression notification an
// AGGDEBUFF-type effect triggers: an attackable target is notified of the
// caster's aggression at the skill's power, while a playable target is
// provoked into attacking the caster if it was already targeting it, or
// retargeted onto the caster otherwise.
func fireAggressionEvent(caster Creature, effected Creature, def modelskill.Definition) {
	if effected.Attackable() {
		effected.NotifyAggression(caster, int(def.Power))
		return
	}
	if effected.Kind().Playable() {
		r := effected
		var tracked world.Tracked
		if caster != nil {
			tracked = caster
		}
		current, _ := r.CurrentTarget().(Actor)
		if sameObject(current, caster) {
			r.AttackTarget(tracked)
		} else {
			r.SetTarget(tracked)
		}
	}
}
