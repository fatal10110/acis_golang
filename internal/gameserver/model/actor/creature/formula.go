package creature

import (
	"math"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// FormulaActor is the live actor surface needed to resolve skill formula
// inputs before they are passed to the pure formula package.
type FormulaActor interface {
	Position() (x, y, z int)
	Heading() int
	Level() int

	STR() int
	CON() int
	DEX() int
	INT() int
	WIT() int
	MEN() int

	PAtk() float64
	PDef() float64
	MAtk() float64
	MDef() float64
	MagicCriticalRate() float64
	AttackType() item.WeaponType

	SoulshotCharged() bool
	SpiritshotCharged() bool
	BlessedSpiritshotCharged() bool

	CalcStat(stat.Stat, float64) float64
	RandomDamageSpread() int
	Roll(int) int
}

// ResolvePhysicalSkillInput builds a physical-skill damage input from the
// caster/target pair. raceMul is supplied by NPC targets whose template race
// has a matching attack/resistance stat pair.
func ResolvePhysicalSkillInput(caster any, target FormulaActor, def modelskill.Definition, pvp bool, raceMul float64) (formulas.PhysicalSkillInput, bool) {
	attacker, ok := caster.(FormulaActor)
	if !ok || attacker == nil || target == nil {
		return formulas.PhysicalSkillInput{}, false
	}
	soulshot := attacker.SoulshotCharged()
	skillPower := float64(def.Power)
	if soulshot && def.SoulShotBoost > 0 {
		skillPower *= float64(def.SoulShotBoost)
	}
	pvpMul := 1.0
	if pvp {
		pvpMul = attacker.CalcStat(stat.PvPPhysSkillDmg, 1)
	}
	return formulas.PhysicalSkillInput{
		AttackPower:   attacker.PAtk(),
		SkillPower:    skillPower,
		Defence:       Positive(target.PDef()),
		Crit:          PhysicalSkillCrit(attacker, def),
		SoulShot:      soulshot,
		RandomMul:     RandomDamageMultiplier(attacker, def),
		ElementalMul:  ElementalSkillModifier(target, def),
		RaceMul:       raceMul,
		WeaponVulnMul: WeaponVulnerability(target, attacker.AttackType()),
		PvPMul:        pvpMul,
	}, true
}

// ResolveMagicDamageInput builds a magic-damage input from the caster/target
// pair.
func ResolveMagicDamageInput(caster any, target FormulaActor, def modelskill.Definition, pvp bool) (formulas.MagicDamageInput, bool) {
	attacker, ok := caster.(FormulaActor)
	if !ok || attacker == nil || target == nil {
		return formulas.MagicDamageInput{}, false
	}
	sps, bsps := SpiritshotFlags(attacker)
	return formulas.MagicDamageInput{
		MAtk:            attacker.MAtk(),
		MDef:            Positive(target.MDef()),
		SkillPower:      float64(def.Power),
		PvPMul:          MagicPvPMul(attacker, def, pvp),
		ElementalMul:    ElementalSkillModifier(target, def),
		MagicCrit:       formulas.MCritSucceeds(int(attacker.MagicCriticalRate()), attacker.Roll(1000)),
		SoulShot:        sps,
		BlessedSoulShot: bsps,
	}, true
}

// ResolveBlowInput builds a blow-damage input from the caster/target pair.
func ResolveBlowInput(caster any, target FormulaActor, def modelskill.Definition, pvp bool) (formulas.BlowInput, bool) {
	attacker, ok := caster.(FormulaActor)
	if !ok || attacker == nil || target == nil {
		return formulas.BlowInput{}, false
	}
	soulshot := attacker.SoulshotCharged()
	skillPower := float64(def.Power)
	if soulshot && def.SoulShotBoost > 0 {
		skillPower *= float64(def.SoulShotBoost)
	}
	pvpMul := 1.0
	if pvp {
		pvpMul = attacker.CalcStat(stat.PvPPhysSkillDmg, 1)
	}
	return formulas.BlowInput{
		AttackPower:       attacker.PAtk(),
		SkillPower:        skillPower,
		Defence:           Positive(target.PDef()),
		SoulShot:          soulshot,
		IsPvP:             pvp,
		RandomMul:         float64(95+attacker.Roll(11)) / 100,
		PosMul:            PositionMultiplierFrom(target, attacker, true),
		PvPMul:            pvpMul,
		CritDamageMul:     attacker.CalcStat(stat.CriticalDamage, 1),
		CritDamagePosMul:  (attacker.CalcStat(stat.CriticalDamagePos, 1)-1)/2 + 1,
		CritVulnMul:       target.CalcStat(stat.CritVuln, 1),
		DaggerVulnMul:     target.CalcStat(stat.DaggerWpnVuln, 1),
		CritDamageAddBase: attacker.CalcStat(stat.CriticalDamageAdd, 0),
	}, true
}

// ResolveManaDamageInput builds a mana-damage input from the caster/target
// pair.
func ResolveManaDamageInput(caster any, target FormulaActor, maxMP float64, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	attacker, ok := caster.(FormulaActor)
	if !ok || attacker == nil || target == nil {
		return formulas.ManaDamageInput{}, false
	}
	sps, bsps := SpiritshotFlags(attacker)
	return formulas.ManaDamageInput{
		MAtk:            attacker.MAtk(),
		MDef:            Positive(target.MDef()),
		SkillPower:      float64(def.Power),
		TargetMaxMp:     maxMP,
		SoulShot:        sps,
		BlessedSoulShot: bsps,
		VulnMul:         SkillVulnerability(target, def.SkillType, def),
	}, true
}

// ResolveSkillSuccessInput builds the effect-landing input from the
// caster/target pair.
func ResolveSkillSuccessInput(caster any, target FormulaActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if target == nil {
		return formulas.SkillSuccessInput{}, false
	}
	if def.IgnoreResists {
		return formulas.SkillSuccessInput{
			BaseChance:    float64(def.BaseLandRate),
			IgnoreResists: true,
			Shield:        shield,
		}, true
	}
	attacker, ok := caster.(FormulaActor)
	if !ok || attacker == nil {
		return formulas.SkillSuccessInput{}, false
	}
	return formulas.SkillSuccessInput{
		BaseChance:    float64(def.BaseLandRate),
		StatModifier:  SkillStatModifier(target, def.EffectType, def.Magic),
		VulnModifier:  SkillVulnerability(target, def.EffectType, def),
		MAtkModifier:  SkillMAtkModifier(target, attacker, def, bss),
		LevelModifier: SkillLevelModifier(target.Level(), attacker.Level(), def),
		IgnoreResists: def.IgnoreResists,
		Shield:        shield,
	}, true
}

// PhysicalSkillCrit reports whether a physical skill crits for attacker.
func PhysicalSkillCrit(attacker FormulaActor, def modelskill.Definition) bool {
	if attacker == nil || def.BaseCritRate <= 0 {
		return false
	}
	rate := float64(def.BaseCritRate) * 10 * statbonus.STRBonus[attacker.STR()]
	return formulas.CritSucceeds(rate, attacker.Roll(1000))
}

// RandomDamageMultiplier returns the attacker's random damage multiplier for
// physical skills.
func RandomDamageMultiplier(attacker FormulaActor, def modelskill.Definition) float64 {
	if attacker == nil || SkillTypeKey(def.EffectType) == "CHARGEDAM" {
		return 1
	}
	spread := attacker.RandomDamageSpread()
	if spread <= 0 {
		return 1
	}
	return 1 + float64(attacker.Roll(2*spread+1)-spread)/100
}

// SpiritshotFlags returns the magic-shot flags a damage formula expects.
func SpiritshotFlags(attacker FormulaActor) (sps, bsps bool) {
	if attacker == nil {
		return false, false
	}
	bsps = attacker.BlessedSpiritshotCharged()
	if bsps {
		return false, true
	}
	return attacker.SpiritshotCharged(), false
}

// MagicPvPMul returns the caster's PvP damage multiplier for a magic-damage
// formula, or 1 outside playable-vs-playable combat.
func MagicPvPMul(attacker FormulaActor, def modelskill.Definition, pvp bool) float64 {
	if !pvp || attacker == nil {
		return 1
	}
	if def.Magic {
		return attacker.CalcStat(stat.PvPMagicalDmg, 1)
	}
	return attacker.CalcStat(stat.PvPPhysSkillDmg, 1)
}

// Playable reports whether v is a player-controlled actor for PvP formula
// gating.
func Playable(v any) bool {
	p, ok := v.(interface{ Playable() bool })
	return ok && p.Playable()
}

// PositionMultiplierFrom returns the target-facing position multiplier for a
// physical or blow attack by attacker.
func PositionMultiplierFrom(target, attacker FormulaActor, crit bool) float64 {
	if target == nil || attacker == nil {
		return 1
	}
	tx, ty, tz := target.Position()
	ax, ay, az := attacker.Position()
	facing := location.OrientedLocation{
		Location: location.Location{X: tx, Y: ty, Z: tz},
		Heading:  formulaHeading(target),
	}
	return formulas.PosMul(facing.IsBehind(location.Location{X: ax, Y: ay, Z: az}), facing.IsInFrontOf(location.Location{X: ax, Y: ay, Z: az}), crit)
}

func formulaHeading(actor FormulaActor) int {
	if h, ok := actor.(interface{ CurrentHeading() int }); ok {
		return h.CurrentHeading()
	}
	return actor.Heading()
}

// SkillStatModifier returns the target attribute modifier used by effect
// landing rates.
func SkillStatModifier(target FormulaActor, typ string, magic bool) float64 {
	if target == nil {
		return 1
	}
	switch SkillTypeKey(typ) {
	case "STUN", "BLEED", "POISON":
		return math.Max(0, 2-math.Sqrt(statbonus.CONBonus[target.CON()]))
	case "SLEEP", "DEBUFF", "WEAKNESS", "ERASE", "ROOT", "MUTE", "FEAR", "BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "PARALYZE":
		if magic {
			return math.Max(0, 2-math.Sqrt(statbonus.MENBonus[target.MEN()]))
		}
	}
	return 1
}

// SkillVulnerability returns the target's skill-type vulnerability multiplier,
// including elemental resistance as the base.
func SkillVulnerability(target FormulaActor, typ string, def modelskill.Definition) float64 {
	if target == nil {
		return 1
	}
	base := math.Sqrt(ElementalSkillModifier(target, def))
	switch SkillTypeKey(typ) {
	case "BLEED":
		return target.CalcStat(stat.BleedVuln, base)
	case "POISON":
		return target.CalcStat(stat.PoisonVuln, base)
	case "STUN":
		return target.CalcStat(stat.StunVuln, base)
	case "PARALYZE":
		return target.CalcStat(stat.ParalyzeVuln, base)
	case "ROOT":
		return target.CalcStat(stat.RootVuln, base)
	case "SLEEP":
		return target.CalcStat(stat.SleepVuln, base)
	case "MUTE", "FEAR", "BETRAY", "AGGDEBUFF", "AGGREDUCE_CHAR", "ERASE", "CONFUSION":
		return target.CalcStat(stat.DerangementVuln, base)
	case "DEBUFF", "WEAKNESS":
		return target.CalcStat(stat.DebuffVuln, base)
	case "CANCEL":
		return target.CalcStat(stat.CancelVuln, base)
	default:
		return base
	}
}

// SkillMAtkModifier returns the magic-attack-vs-defence term used by effect
// landing rates.
func SkillMAtkModifier(target, attacker FormulaActor, def modelskill.Definition, bss bool) float64 {
	if !def.Magic || target == nil || attacker == nil {
		return 1
	}
	mDef := Positive(target.MDef())
	mAtk := attacker.MAtk()
	if bss {
		mAtk *= 4
	}
	return math.Sqrt(mAtk) / mDef * 11
}

// SkillLevelModifier returns the level-difference term used by effect landing
// rates.
func SkillLevelModifier(targetLevel, casterLevel int, def modelskill.Definition) float64 {
	if def.LevelDepend == 0 {
		return 1
	}
	level := casterLevel
	if def.MagicLevel > 0 {
		level = def.MagicLevel
	}
	delta := level + def.LevelDepend - targetLevel
	scale := 0.005
	if delta < 0 {
		scale = 0.01
	}
	return 1 + scale*float64(delta)
}

// ElementalSkillModifier returns target's resistance multiplier for def's
// element.
func ElementalSkillModifier(target FormulaActor, def modelskill.Definition) float64 {
	if target == nil {
		return 1
	}
	s, ok := ElementResistanceStat(def.Element)
	if !ok {
		return 1
	}
	return target.CalcStat(s, 1)
}

// ElementResistanceStat returns the stat used for element resistance.
func ElementResistanceStat(element modelskill.Element) (stat.Stat, bool) {
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

// WeaponVulnerability returns target's vulnerability multiplier for an
// attacker's weapon type.
func WeaponVulnerability(target FormulaActor, weapon item.WeaponType) float64 {
	if target == nil {
		return 1
	}
	switch weapon {
	case item.WeaponSword:
		return target.CalcStat(stat.SwordWpnVuln, 1)
	case item.WeaponBlunt:
		return target.CalcStat(stat.BluntWpnVuln, 1)
	case item.WeaponDagger:
		return target.CalcStat(stat.DaggerWpnVuln, 1)
	case item.WeaponBow:
		return target.CalcStat(stat.BowWpnVuln, 1)
	case item.WeaponPole:
		return target.CalcStat(stat.PoleWpnVuln, 1)
	case item.WeaponDual:
		return target.CalcStat(stat.DualWpnVuln, 1)
	case item.WeaponDualFist:
		return target.CalcStat(stat.DualFistWpnVuln, 1)
	case item.WeaponBigSword:
		return target.CalcStat(stat.BigSwordWpnVuln, 1)
	case item.WeaponBigBlunt:
		return target.CalcStat(stat.BigBluntWpnVuln, 1)
	default:
		return 1
	}
}

// Positive returns v unless it is zero or negative, in which case it returns 1.
func Positive(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

// SkillTypeKey normalizes skill and effect type names for formula dispatch.
func SkillTypeKey(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}
