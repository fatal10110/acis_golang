package effect

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// increaseChargesStart adds the template's charge amount, capped at the
// template's count (repurposed here as the max-charges cap, not a tick
// count), matching the reference's one-shot onStart call into
// Player.increaseCharges. It always reports success: whether the target
// was already at the cap is the target method's own no-op/system-message
// concern, not this effect's.
func increaseChargesStart(e *Effect) bool {
	target, ok := asPlayer(e.Effected)
	if !ok {
		return false
	}
	target.IncreaseCharges(int(e.Template.Value), e.Template.Count)
	return true
}

func targetMeStart(e *Effect) bool {
	target, ok := asPlayer(e.Effected)
	if !ok {
		return false
	}
	var effector world.Tracked
	if e.Effector != nil {
		effector = e.Effector
	}
	if target.CurrentTarget() == effector {
		target.TryToAttack(effector)
	} else {
		target.SetTarget(effector)
	}
	return true
}

func bluffStart(e *Effect) bool {
	if e.Effected.RaidRelated() || e.Effected.BluffExempt() || e.Effector == nil {
		return false
	}
	e.Effected.SetHeading(e.Effector.Heading())
	return true
}

func charmOfCourageStart(e *Effect) bool {
	return isPlayer(e.Effected)
}

func charmOfLuckExit(e *Effect) {
	if target, ok := asPlayer(e.Effected); ok {
		target.StopCharmOfLuck(e)
	}
}

func phoenixBlessExit(e *Effect) {
	if target, ok := asPlayer(e.Effected); ok {
		target.StopPhoenixBlessing(e)
	}
}

// cancelStart strips a random subset of the effected actor's active
// non-toggle, non-debuff effects, up to e.Skill.MaxNegatedEffects (0 means
// unlimited). Each candidate rolls independently against
// formulas.EffectCancelSuccessRate.
//
// The classification checked against the protected-marker exemption list is
// this effect's own tag (e.ClassTag()), which is always the cancel
// classification and never matches any of the four protected markers
// (courage/luck charms, noblesse and protection blessings) — so that
// exemption can never actually trigger here, and those markers remain
// cancellable through this path even though the check reads as if it
// guards them. This is the required behavior; do not "fix" it by checking
// the candidate's tag instead.
func cancelStart(e *Effect) bool {
	if e.Effected.Dead() {
		return false
	}
	list := e.Effected.EffectList()
	if list == nil {
		return true
	}
	if effectNotCancellable[strings.ToUpper(e.ClassTag())] {
		return true
	}

	vuln := e.Effected.CancelVulnerability(e.ClassTag())

	count := e.Skill.MaxNegatedEffects
	candidates := list.All()
	shuffleEffects(candidates)

	for _, cand := range candidates {
		if cand.Skill.Toggle || cand.Skill.Debuff {
			continue
		}

		rate := formulas.EffectCancelSuccessRate(e.Skill.MagicLevel, cand.Skill.MagicLevel, cand.Template.Time, e.Template.EffectPower, vuln)
		if formulas.CancelSucceeds(float64(rate), rnd.Get(100)) {
			list.Remove(cand)
		}

		if count > 0 {
			count--
			if count == 0 {
				break
			}
		}
	}
	return true
}

// effectNotCancellable are effect classification tags that appear to be
// exempt from cancelStart's strip loop; see cancelStart's doc comment for
// why the exemption never actually applies there.
var effectNotCancellable = map[string]bool{
	"CHARM_OF_COURAGE":    true,
	"CHARM_OF_LUCK":       true,
	"NOBLESSE_BLESSING":   true,
	"PROTECTION_BLESSING": true,
}

// shuffleEffects randomizes candidates in place (Fisher-Yates) so a capped
// cancel/dispel loop doesn't always prefer the same array position.
func shuffleEffects(candidates []*Effect) {
	for i := len(candidates) - 1; i > 0; i-- {
		j := rnd.Get(i + 1)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
}

// negateStart strips every active effect on the effected actor that's owned
// by one of e.Skill.NegateIDs, plus every active effect whose classification
// matches one of e.Skill.NegateTypes and whose abnormal level (per-effect
// when its owning skill sets EffectType, per-skill otherwise) is within
// e.Skill.NegateLevel — or any level when NegateLevel is -1.
func negateStart(e *Effect) bool {
	list := e.Effected.EffectList()
	if list == nil {
		return true
	}

	for _, id := range e.Skill.NegateIDs {
		if id == 0 {
			continue
		}
		for _, cand := range list.All() {
			if int(cand.Skill.ID) == id {
				list.Remove(cand)
			}
		}
	}

	for _, negType := range e.Skill.NegateTypes {
		negType = strings.ToUpper(strings.TrimSpace(negType))
		for _, cand := range list.All() {
			if !negateTypeMatches(cand.Skill, negType) {
				continue
			}
			if !negateLevelAllows(cand.Skill, e.Skill.NegateLevel) {
				continue
			}
			list.Remove(cand)
		}
	}
	return true
}

// negateTypeMatches reports whether candidate's classification (its own
// skill type, or its own effect-type tag when set) matches negType.
func negateTypeMatches(candidate Skill, negType string) bool {
	if strings.EqualFold(candidate.SkillType, negType) {
		return true
	}
	return candidate.EffectType != "" && strings.EqualFold(candidate.EffectType, negType)
}

// negateLevelAllows reports whether candidate's applicable abnormal level
// (EffectAbnormalLevel when its own EffectType is set, AbnormalLevel
// otherwise) is within negateLvl, or negateLvl is -1 (unrestricted).
func negateLevelAllows(candidate Skill, negateLvl int) bool {
	if negateLvl == -1 {
		return true
	}
	if candidate.EffectType != "" && candidate.EffectAbnormalLevel >= 0 && candidate.EffectAbnormalLevel <= negateLvl {
		return true
	}
	return candidate.AbnormalLevel >= 0 && candidate.AbnormalLevel <= negateLvl
}

// fusionAction is a fusion effect's periodic tick: it never ends on its own
// action timer, only when its Time runs out or IncreaseEffect/
// DecreaseForce removes it.
func fusionAction(*Effect) bool {
	return true
}

// IncreaseEffect grows a live fusion effect by one level, up to maxLevel.
// It removes this instance from list and, unless it was already at
// maxLevel, asks reapply to install a fresh instance at the grown level —
// exactly what constructing a new effect at that level in this one's place
// would produce. Doing nothing at maxLevel (rather than reapplying at the
// same level) matches the reference: the growth attempt is a plain no-op
// once the effect is already maxed out.

func chanceSkillTriggerStart(e *Effect) bool {
	e.Effected.AddChanceTrigger(e)
	return true
}

func chanceSkillTriggerExit(e *Effect) {
	e.Effected.RemoveChanceTrigger(e)
}

// spoilRoll is the upper bound of the uniform random draw a spoil effect's
// magic-resist roll compares against, matching the SPOIL skill-type
// handler's own roll.
const spoilRoll = 10000

// spoilStart rolls a magic-resist check against a live, unspoiled monster
// target and marks it spoiled by the caster on success. It always reports
// success once the roll is attempted, regardless of the roll's outcome.

func cancelDebuffStart(e *Effect) bool {
	if !isPlayer(e.Effected) || e.Effected.Dead() {
		return false
	}
	list := e.Effected.EffectList()
	if list == nil {
		return true
	}

	vuln := e.Effected.CancelVulnerability(e.ClassTag())

	candidates := list.All()
	count := cancelDebuffPass(list, candidates, e.Skill.MagicLevel, vuln, e.Skill.MaxNegatedEffects)
	if count != 0 {
		cancelDebuffPass(list, candidates, e.Skill.MagicLevel, vuln, count)
	}
	return true
}

// cancelDebuffPass runs one reverse-order sweep of candidates, stripping
// dispellable debuffs against an independent roll each, up to count
// removals (0 meaning unlimited); it returns the remaining count.
func cancelDebuffPass(list *List, candidates []*Effect, cancelLvl int, vuln float64, count int) int {
	lastCanceledSkillID := modelskill.ID(0)
	for i := len(candidates) - 1; i >= 0; i-- {
		cand := candidates[i]
		if !cand.Skill.Debuff || !cand.Skill.CanBeDispelled {
			continue
		}
		if cand.Skill.ID == lastCanceledSkillID {
			list.Remove(cand)
			continue
		}

		// Template.Time (the candidate's full configured duration) stands in for the
		// reference effect's remaining duration (period-elapsed): this port has no
		// live elapsed-time tracking per effect yet, so "remaining" always reads as
		// "full". Revisit once effects track elapsed time.
		rate := formulas.EffectCancelDebuffSuccessRate(cancelLvl, cand.Skill.MagicLevel, cand.Template.Time, vuln)
		if !formulas.CancelSucceeds(float64(rate), rnd.Get(100)) {
			continue
		}

		lastCanceledSkillID = cand.Skill.ID
		list.Remove(cand)
		count--
		if count == 0 {
			break
		}
	}
	return count
}
