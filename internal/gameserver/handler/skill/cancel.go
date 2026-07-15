package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// effectNotCancellable are effect classification tags a cancel-family skill
// can never strip, regardless of roll.
var effectNotCancellable = map[string]bool{
	"CHARM_OF_COURAGE":    true,
	"CHARM_OF_LUCK":       true,
	"NOBLESSE_BLESSING":   true,
	"PROTECTION_BLESSING": true,
}

type cancelTarget interface {
	effectListTarget
	Dead() bool
	Level() int
}

// cancelVulnerabilitySource optionally supplies the target's already-
// resolved cancel-vulnerability multiplier for skillType; a target without
// one is treated as unmodified (1.0).
type cancelVulnerabilitySource interface {
	CancelVulnerability(skillType string) float64
}

type cancelHandler struct{}

func (cancelHandler) Types() []string { return []string{"CANCEL", "MAGE_BANE", "WARRIOR_BANE"} }

// Use rolls to strip a random subset of each target's active non-toggle,
// non-debuff effects, then optionally refreshes the caster's own
// self-targeted copy of this skill's effects.
func (cancelHandler) Use(cast Cast) {
	skillType := skillTypeKey(cast.Skill.SkillType)
	minRate, maxRate := 25, 75
	if skillType != "CANCEL" {
		minRate, maxRate = 40, 95
	}

	for _, obj := range cast.Targets {
		target, ok := obj.(cancelTarget)
		if !ok || target.Dead() {
			continue
		}
		cancelOne(cast, target, skillType, minRate, maxRate)
	}

	applySelfEffects(cast.Caster, cast.Skill)
}

func cancelOne(cast Cast, target cancelTarget, skillType string, minRate, maxRate int) {
	vuln := 1.0
	if v, ok := any(target).(cancelVulnerabilitySource); ok {
		vuln = v.CancelVulnerability(skillType)
	}

	diffLevel := cast.Skill.MagicLevel - target.Level()
	count := cast.Skill.MaxNegatedEffects

	list := target.EffectList()
	effects := list.All()
	shuffleEffects(effects)

	for _, e := range effects {
		if e.Skill.Toggle || e.Skill.Debuff {
			continue
		}
		if effectNotCancellable[strings.ToUpper(e.ClassTag())] {
			continue
		}

		switch skillType {
		case "MAGE_BANE":
			switch strings.ToLower(e.Template.StackType) {
			case "casting_time_down", "ma_up":
			default:
				continue
			}
		case "WARRIOR_BANE":
			switch strings.ToLower(e.Template.StackType) {
			case "attack_time_down", "speed_up":
			default:
				continue
			}
		}

		rate := formulas.CancelSuccessRate(e.Template.Time, diffLevel, float64(cast.Skill.Power), vuln, minRate, maxRate)
		if formulas.CancelSucceeds(rate, rnd.Get(100)) {
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
