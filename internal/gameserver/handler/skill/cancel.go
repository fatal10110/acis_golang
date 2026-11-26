package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type cancelHandler struct{}

func (cancelHandler) Types() []modelskill.SkillType {
	return []modelskill.SkillType{modelskill.SkillTypeCancel, modelskill.SkillTypeMageBane, modelskill.SkillTypeWarriorBane}
}

// Use rolls to strip a random subset of each target's active non-toggle,
// non-debuff effects, then optionally refreshes the caster's own
// self-targeted copy of this skill's effects.
func (cancelHandler) Use(cast Cast) {
	skillType := skillTypeKey(cast.Skill.SkillType)
	minRate, maxRate := 25, 75
	if skillType != modelskill.SkillTypeCancel {
		minRate, maxRate = 40, 95
	}

	for _, obj := range cast.Targets {
		target, ok := asEffected(obj)
		if !ok || target.Dead() {
			continue
		}
		cancelOne(cast, target, skillType, minRate, maxRate)
	}

	applySelfEffects(cast, cast.Skill)
	dischargeSpiritshot(cast)
}

// cancelOne rolls each eligible effect of target against the cast's rate; the
// roll comes from the caster's combat random source (see landingRoll), so a
// pinned caster decides every strip.
func cancelOne(cast Cast, target effect.Actor, skillType modelskill.SkillType, minRate, maxRate int) {
	vuln := target.CancelVulnerability(string(skillType))

	diffLevel := cast.Skill.MagicLevel - target.Level()
	count := cast.Skill.Extra().MaxNegatedEffects

	list := target.EffectList()
	effects := list.All()
	shuffleEffects(effects)

	for _, e := range effects {
		if e.Skill.Toggle || e.Skill.Debuff {
			continue
		}
		if effect.NotCancellable(e.ClassTag()) {
			continue
		}

		switch skillType {
		case modelskill.SkillTypeMageBane:
			switch strings.ToLower(e.Template.StackType) {
			case "casting_time_down", "ma_up":
			default:
				continue
			}
		case modelskill.SkillTypeWarriorBane:
			switch strings.ToLower(e.Template.StackType) {
			case "attack_time_down", "speed_up":
			default:
				continue
			}
		}

		rate := formulas.CancelSuccessRate(e.Period(), diffLevel, float64(cast.Skill.Power), vuln, minRate, maxRate)
		if formulas.CancelSucceeds(rate, landingRoll(cast.Caster)) {
			list.Remove(e)
		}

		if count > 0 {
			count--
			if count == 0 {
				break
			}
		}
	}
}

func shuffleEffects(effects []*effect.Effect) {
	for i := len(effects) - 1; i > 0; i-- {
		j := rnd.Get(i + 1)
		effects[i], effects[j] = effects[j], effects[i]
	}
}
