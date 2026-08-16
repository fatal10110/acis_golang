package npc

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// SkillSuccessInput returns the effect-landing roll input for def cast
// against h.
func (h *Hostile) SkillSuccessInput(caster creature.DeathActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if h == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return creature.ResolveSkillSuccessInput(caster, h, def, bss, shield)
}

func (h *Hostile) EffectSuccessInput(caster creature.DeathActor, def modelskill.Definition, tmpl modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if tmpl.EffectType == "" {
		return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
	}
	if strings.EqualFold(tmpl.EffectType, "CANCEL") {
		return formulas.SkillSuccessInput{BaseChance: 100, IgnoreResists: true, Shield: shield}, true
	}
	def.EffectType = tmpl.EffectType
	def.IgnoreResists = false
	in, ok := h.SkillSuccessInput(caster, def, bss, shield)
	in.BaseChance = tmpl.EffectPower
	return in, ok
}

// MAtk returns this NPC's magic attack stat.
func (h *Hostile) MAtk() float64 {
	return h.calcStat(stat.MagicAttack, positiveStat(h.Instance.Template.MAtk))
}

// MDef returns this NPC's magic defence stat.
func (h *Hostile) MDef() float64 {
	return h.calcStat(stat.MagicDefence, positiveStat(h.Instance.Template.MDef))
}

func positiveStat(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}
