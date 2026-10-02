package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// SkillSuccessInput returns the effect-landing roll input for def cast
// against h.
func (h *Hostile) SkillSuccessInput(caster creature.FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if h == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return creature.ResolveSkillSuccessInput(caster, h, def, bss, shield)
}

func (h *Hostile) EffectSuccessInput(caster creature.FormulaActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if h == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return creature.ResolveEffectSuccessInput(caster, h, def, tmpl, bss, shield)
}

// MAtk returns this NPC's magic attack stat, truncated to a whole number.
func (h *Hostile) MAtk() float64 {
	return math.Trunc(h.calcStat(stat.MagicAttack, positiveStat(h.Instance.Template.MAtk)))
}

// MDef returns this NPC's magic defence stat, finalized from the template
// base scaled by the raid defence multiplier while raid related, and
// truncated to a whole number.
func (h *Hostile) MDef() float64 {
	return math.Trunc(h.calcStat(stat.MagicDefence, positiveStat(h.Instance.Template.MDef)*h.raidBaseMultipliers().Defence))
}

func positiveStat(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}
