package npc

import (
	"math"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

type skillSuccessCaster interface {
	MAtk() float64
	Level() int
}

// SkillSuccessInput returns the effect-landing roll input for def cast
// against h.
func (h *Hostile) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if h == nil {
		return formulas.SkillSuccessInput{}, false
	}
	if def.IgnoreResists {
		return formulas.SkillSuccessInput{
			BaseChance:    float64(def.BaseLandRate),
			IgnoreResists: true,
			Shield:        shield,
		}, true
	}
	attacker, ok := caster.(skillSuccessCaster)
	if !ok || attacker == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return formulas.SkillSuccessInput{
		BaseChance:    float64(def.BaseLandRate),
		StatModifier:  h.skillStatModifier(def.EffectType, def.Magic),
		VulnModifier:  h.skillVulnerability(def.EffectType, def),
		MAtkModifier:  h.skillMAtkModifier(attacker, def, bss),
		LevelModifier: h.skillLevelModifier(attacker, def),
		IgnoreResists: def.IgnoreResists,
		Shield:        shield,
	}, true
}

// MAtk returns this NPC's magic attack stat.
func (h *Hostile) MAtk() float64 {
	return h.calcStat(stat.MagicAttack, positiveStat(h.Instance.Template.MAtk))
}

// MDef returns this NPC's magic defence stat.
func (h *Hostile) MDef() float64 {
	return h.calcStat(stat.MagicDefence, positiveStat(h.Instance.Template.MDef))
}

func (h *Hostile) skillStatModifier(typ string, magic bool) float64 {
	switch strings.ToUpper(typ) {
	case "STUN", "BLEED", "POISON":
		return math.Max(0, 2-math.Sqrt(statbonus.CONBonus[statbonus.ClampIndex(h.Instance.Template.CON)]))
	case "SLEEP", "DEBUFF", "WEAKNESS", "ERASE", "ROOT", "MUTE", "FEAR", "BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "PARALYZE":
		if magic {
			return math.Max(0, 2-math.Sqrt(statbonus.MENBonus[statbonus.ClampIndex(h.Instance.Template.MEN)]))
		}
	}
	return 1
}

func (h *Hostile) skillVulnerability(typ string, def modelskill.Definition) float64 {
	base := math.Sqrt(h.elementalSkillModifier(def))
	switch strings.ToUpper(typ) {
	case "BLEED":
		return h.calcStat(stat.BleedVuln, base)
	case "POISON":
		return h.calcStat(stat.PoisonVuln, base)
	case "STUN":
		return h.calcStat(stat.StunVuln, base)
	case "PARALYZE":
		return h.calcStat(stat.ParalyzeVuln, base)
	case "ROOT":
		return h.calcStat(stat.RootVuln, base)
	case "SLEEP":
		return h.calcStat(stat.SleepVuln, base)
	case "MUTE", "FEAR", "BETRAY", "AGGDEBUFF", "AGGREDUCE_CHAR", "ERASE", "CONFUSION":
		return h.calcStat(stat.DerangementVuln, base)
	case "DEBUFF", "WEAKNESS":
		return h.calcStat(stat.DebuffVuln, base)
	case "CANCEL":
		return h.calcStat(stat.CancelVuln, base)
	default:
		return base
	}
}

func (h *Hostile) skillMAtkModifier(attacker skillSuccessCaster, def modelskill.Definition, bss bool) float64 {
	if !def.Magic {
		return 1
	}
	mAtk := positiveStat(attacker.MAtk())
	if bss {
		mAtk *= 4
	}
	return math.Sqrt(mAtk) / positiveStat(h.MDef()) * 11
}

func (h *Hostile) skillLevelModifier(attacker skillSuccessCaster, def modelskill.Definition) float64 {
	if def.LevelDepend == 0 {
		return 1
	}
	level := positiveLevel(attacker.Level())
	if def.MagicLevel > 0 {
		level = def.MagicLevel
	}
	delta := level + def.LevelDepend - positiveLevel(h.Level())
	scale := 0.005
	if delta < 0 {
		scale = 0.01
	}
	return 1 + scale*float64(delta)
}

func (h *Hostile) elementalSkillModifier(def modelskill.Definition) float64 {
	s, ok := elementResistanceStat(def.Element)
	if !ok {
		return 1
	}
	return h.calcStat(s, 1)
}

func elementResistanceStat(element modelskill.Element) (stat.Stat, bool) {
	switch element {
	case modelskill.ElementWind:
		return stat.WindRes, true
	case modelskill.ElementFire:
		return stat.FireRes, true
	case modelskill.ElementWater:
		return stat.WaterRes, true
	case modelskill.ElementEarth:
		return stat.EarthRes, true
	case modelskill.ElementHoly:
		return stat.HolyRes, true
	case modelskill.ElementDark:
		return stat.DarkRes, true
	case modelskill.ElementValakas:
		return stat.ValakasRes, true
	default:
		return 0, false
	}
}

func positiveStat(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

func positiveLevel(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}
