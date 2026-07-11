package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// atkAccuracy finalizes accuracy from DEX and level, with a small flat
// bonus for summons.
type atkAccuracy struct{ fixed }

// AtkAccuracy is the shared instance every creature's calculation chain
// attaches for stat.AccuracyCombat.
var AtkAccuracy = atkAccuracy{fixed{stat.AccuracyCombat}}

func (atkAccuracy) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	level := a.Level()

	value += statbonus.BaseEvasionAccuracy[a.DEX()] + float64(level)
	if a.IsSummon() {
		if level < 60 {
			value += 4
		} else {
			value += 5
		}
	}
	return value
}

// atkCritical finalizes critical rate from DEX (non-summons only) as a
// per-mille value (multiplied by 10 to convert from percent).
type atkCritical struct{ fixed }

var AtkCritical = atkCritical{fixed{stat.CriticalRate}}

func (atkCritical) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	if !a.IsSummon() {
		value *= statbonus.DEXBonus[a.DEX()]
	}
	return value * 10
}

// atkEvasion finalizes evasion rate from DEX and level.
type atkEvasion struct{ fixed }

var AtkEvasion = atkEvasion{fixed{stat.EvasionRate}}

func (atkEvasion) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	return value + statbonus.BaseEvasionAccuracy[a.DEX()] + float64(a.Level())
}

// mAtkCritical finalizes magic critical rate from WIT, except for an
// empty-handed player (who gets none).
type mAtkCritical struct{ fixed }

var MAtkCritical = mAtkCritical{fixed{stat.MCriticalRate}}

func (mAtkCritical) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	p, isPlayer := effector.(PlayerActor)
	if !isPlayer || p.HasWeaponEquipped() {
		return value * statbonus.WITBonus[a.WIT()]
	}
	return value
}

// mAtkMod finalizes M.Atk from INT and the level-scaling factor, squaring
// both multipliers.
type mAtkMod struct{ fixed }

var MAtkMod = mAtkMod{fixed{stat.MagicAttack}}

func (mAtkMod) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	intMod := statbonus.INTBonus[a.INT()]
	lvlMod := a.LevelMod()
	return value * (lvlMod * lvlMod) * (intMod * intMod)
}

// mAtkSpeed finalizes magic attack speed from WIT.
type mAtkSpeed struct{ fixed }

var MAtkSpeed = mAtkSpeed{fixed{stat.MagicAttackSpeed}}

func (mAtkSpeed) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.WITBonus[actorOf(effector).WIT()]
}

// mDefMod finalizes M.Def from MEN and the level-scaling factor, with flat
// penalties for a player's worn accessories (fewer accessory slots equipped
// means less magic defense, matching a bare-handed dodge/parry intuition
// the reference server encodes as a direct subtraction per slot).
type mDefMod struct{ fixed }

var MDefMod = mDefMod{fixed{stat.MagicDefence}}

func (mDefMod) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	if p, ok := effector.(PlayerActor); ok {
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
	return value * statbonus.MENBonus[a.MEN()] * a.LevelMod()
}

// pAtkMod finalizes P.Atk from STR and the level-scaling factor.
type pAtkMod struct{ fixed }

var PAtkMod = pAtkMod{fixed{stat.PowerAttack}}

func (pAtkMod) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	return value * statbonus.STRBonus[a.STR()] * a.LevelMod()
}

// pAtkSpeed finalizes physical attack speed from DEX.
type pAtkSpeed struct{ fixed }

var PAtkSpeed = pAtkSpeed{fixed{stat.PowerAttackSpeed}}

func (pAtkSpeed) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.DEXBonus[actorOf(effector).DEX()]
}

// pDefMod finalizes P.Def from the level-scaling factor, with flat
// penalties for a player's worn armor pieces (an unarmored player has
// higher P.Def than one wearing gear that reduces this value — matching
// the reference server's per-slot subtraction, where a mage's chest/legs
// penalty is smaller than a fighter's).
type pDefMod struct{ fixed }

var PDefMod = pDefMod{fixed{stat.PowerDefence}}

func (pDefMod) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	if p, ok := effector.(PlayerActor); ok {
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
	return value * a.LevelMod()
}
