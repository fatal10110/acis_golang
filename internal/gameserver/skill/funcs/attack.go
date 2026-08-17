package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// AtkAccuracy finalizes accuracy from DEX and level, with a small flat
// bonus for summons. It is the shared instance every creature's
// calculation chain attaches for stat.AccuracyCombat.
var AtkAccuracy Func = func(effector stat.Actor, base, value float64) float64 {
	level := effector.Level()

	value += statbonus.BaseEvasionAccuracy[effector.DEX()] + float64(level)
	if effector.IsSummon() {
		if level < 60 {
			value += 4
		} else {
			value += 5
		}
	}
	return value
}

// AtkCritical finalizes critical rate from DEX (non-summons only) as a
// per-mille value (multiplied by 10 to convert from percent).
var AtkCritical Func = func(effector stat.Actor, base, value float64) float64 {
	if !effector.IsSummon() {
		value *= statbonus.DEXBonus[effector.DEX()]
	}
	return value * 10
}

// AtkEvasion finalizes evasion rate from DEX and level.
var AtkEvasion Func = func(effector stat.Actor, base, value float64) float64 {
	return value + statbonus.BaseEvasionAccuracy[effector.DEX()] + float64(effector.Level())
}

// MAtkCritical finalizes magic critical rate from WIT, except for an
// empty-handed player (who gets none).
var MAtkCritical Func = func(effector stat.Actor, base, value float64) float64 {
	p, isPlayer := effector.(stat.PlayerActor)
	if !isPlayer || p.HasWeaponEquipped() {
		return value * statbonus.WITBonus[effector.WIT()]
	}
	return value
}

// MAtkMod finalizes M.Atk from INT and the level-scaling factor, squaring
// both multipliers.
var MAtkMod Func = func(effector stat.Actor, base, value float64) float64 {
	intMod := statbonus.INTBonus[effector.INT()]
	lvlMod := effector.LevelMod()
	return value * (lvlMod * lvlMod) * (intMod * intMod)
}

// MAtkSpeed finalizes magic attack speed from WIT.
var MAtkSpeed Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.WITBonus[effector.WIT()]
}

// MDefMod finalizes M.Def from MEN and the level-scaling factor, with flat
// penalties for a player's worn accessories (fewer accessory slots equipped
// means less magic defense by direct subtraction per slot).
var MDefMod Func = func(effector stat.Actor, base, value float64) float64 {
	if p, ok := effector.(stat.PlayerActor); ok {
		if p.HasEquipped(SlotLFinger) {
			value -= 5
		}
		if p.HasEquipped(SlotRFinger) {
			value -= 5
		}
		if p.HasEquipped(SlotLEar) {
			value -= 9
		}
		if p.HasEquipped(SlotREar) {
			value -= 9
		}
		if p.HasEquipped(SlotNeck) {
			value -= 13
		}
	}
	return value * statbonus.MENBonus[effector.MEN()] * effector.LevelMod()
}

// PAtkMod finalizes P.Atk from STR and the level-scaling factor.
var PAtkMod Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.STRBonus[effector.STR()] * effector.LevelMod()
}

// PAtkSpeed finalizes physical attack speed from DEX.
var PAtkSpeed Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.DEXBonus[effector.DEX()]
}

// PDefMod finalizes P.Def from the level-scaling factor, with flat
// penalties for a player's worn armor pieces (an unarmored player has
// higher P.Def than one wearing gear that reduces this value; mage
// chest/legs penalties are smaller than fighter penalties).
var PDefMod Func = func(effector stat.Actor, base, value float64) float64 {
	if p, ok := effector.(stat.PlayerActor); ok {
		if p.HasEquipped(SlotHead) {
			value -= 12
		}

		if p.HasEquipped(SlotChest) {
			if p.IsMageClass() {
				value -= 15
			} else {
				value -= 31
			}
		}

		// FullBodyArmor already folds in "a chest item is equipped and it
		// occupies the full-body armor slot"; see its doc comment.
		if p.HasEquipped(FullBodyArmor) || p.HasEquipped(SlotLegs) {
			if p.IsMageClass() {
				value -= 8
			} else {
				value -= 18
			}
		}

		if p.HasEquipped(SlotGloves) {
			value -= 8
		}
		if p.HasEquipped(SlotFeet) {
			value -= 7
		}
	}
	return value * effector.LevelMod()
}
