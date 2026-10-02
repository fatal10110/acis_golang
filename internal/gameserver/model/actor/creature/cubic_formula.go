package creature

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ResolveCubicSkillSuccessInput builds the landing input of a cubic's skill
// fired at target. The cubic's own M.Atk (cubicMAtk, quadrupled under the
// owner's blessed spiritshot when bss) drives the magic term against the
// target's M.Def; the stat, vulnerability and level terms read the owner.
// The base chance and resisted type are the skill's landing power and
// landing effect type.
func ResolveCubicSkillSuccessInput(owner, target FormulaActor, def modelskill.Definition, cubicMAtk float64, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if owner == nil || target == nil {
		return formulas.SkillSuccessInput{}, false
	}
	base := def.LandingPower()
	if def.IgnoreResists {
		return formulas.SkillSuccessInput{BaseChance: base, IgnoreResists: true, Shield: shield}, true
	}
	typ := def.LandingEffectType()
	mAtkMod := 1.0
	if def.Magic {
		mAtkMod = formulas.CubicMAtkModifier(cubicMAtk, target.MDef(), bss)
	}
	return formulas.SkillSuccessInput{
		BaseChance:    base,
		StatModifier:  SkillStatModifier(target, typ, def.Magic),
		VulnModifier:  SkillVulnerability(target, typ, def),
		MAtkModifier:  mAtkMod,
		LevelModifier: SkillLevelModifier(target.Level(), owner.Level(), def),
		Shield:        shield,
	}, true
}

// ResolveCubicMagicDamageInput builds a cubic's magic strike input against
// target. crit is the owner's magic critical roll and shield the target's
// block against the owner. With magicFailures set and no perfect block, the
// owner rolls the magic-success check twice, and the half-resist level gap
// is measured from the skill's magic level rather than the owner's level.
func ResolveCubicMagicDamageInput(owner, target FormulaActor, def modelskill.Definition, crit bool, shield formulas.ShieldDefense, magicFailures bool) (formulas.CubicMagicDamageInput, bool) {
	if owner == nil || target == nil {
		return formulas.CubicMagicDamageInput{}, false
	}
	mDef := target.MDef()
	if shield == formulas.ShieldSuccess {
		mDef += target.CalcStat(stat.ShieldDefence, 0)
	}
	in := formulas.CubicMagicDamageInput{
		MDef:         mDef,
		SkillPower:   float64(def.Power),
		MagicCrit:    crit,
		ElementalMul: ElementalSkillModifier(target, def),
		Shield:       shield,
	}
	if shield != formulas.ShieldPerfect && magicFailures {
		rate := formulas.MagicSuccessRate(target.Level(), owner.Level(), def.MagicLevel, def.LevelDepend, owner.WeaponGradePenalty())
		first := formulas.MagicSucceeds(rate, owner.Roll(10000))
		second := false
		if !first {
			second = formulas.MagicSucceeds(rate, owner.Roll(10000))
		}
		in.Failure = formulas.MagicFailureOutcome(true, first, true, second, target.Level()-def.MagicLevel)
	}
	return in, true
}
