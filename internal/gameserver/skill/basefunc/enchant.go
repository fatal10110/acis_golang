package basefunc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// EnchantedItem is what Enchant needs from its Owner: the equipped item's
// enchant level, and — for the weapon P.Atk/M.Atk case — its weapon type
// and crystal grade. A non-weapon (armor, accessory) reports ok=false from
// Weapon. Resolving Owner to something satisfying this interface is the
// equip/inventory system's job, not this package's.
type EnchantedItem interface {
	EnchantLevel() int
	Weapon() (item.WeaponType, bool)
	Crystal() item.CrystalType
}

// Enchant adds an amount driven by the owning item's enchant level: a flat
// per-level bonus to P.Def/M.Def, a crystal-grade-scaled bonus to M.Atk, or
// a crystal-grade-and-weapon-type-scaled bonus to P.Atk for weapons. It
// runs at OrderEnchant, and its Owner must satisfy EnchantedItem.
type Enchant struct{ base }

func NewEnchant(owner any, s stat.Stat, value float64, cond Condition) *Enchant {
	return &Enchant{base{owner, s, OrderEnchant, value, cond}}
}

func (f *Enchant) Calc(effector, effected, skill any, calcBase, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}

	src := f.owner.(EnchantedItem)

	enchant := src.EnchantLevel()
	if enchant <= 0 {
		return value
	}

	overenchant := 0
	if enchant > 3 {
		overenchant = enchant - 3
		enchant = 3
	}

	if f.stat == stat.MagicDefence || f.stat == stat.PowerDefence {
		return value + float64(enchant) + float64(3*overenchant)
	}

	if f.stat == stat.MagicAttack {
		switch src.Crystal() {
		case item.CrystalS:
			value += float64(4*enchant + 8*overenchant)
		case item.CrystalA, item.CrystalB, item.CrystalC:
			value += float64(3*enchant + 6*overenchant)
		case item.CrystalD:
			value += float64(2*enchant + 4*overenchant)
		}
		return value
	}

	wType, isWeapon := src.Weapon()
	if !isWeapon {
		return value
	}

	isBigOrDual := wType == item.WeaponBigBlunt || wType == item.WeaponBigSword || wType == item.WeaponDualFist || wType == item.WeaponDual

	switch src.Crystal() {
	case item.CrystalS:
		switch {
		case wType == item.WeaponBow:
			value += float64(10*enchant + 20*overenchant)
		case isBigOrDual:
			value += float64(6*enchant + 12*overenchant)
		default:
			value += float64(5*enchant + 10*overenchant)
		}
	case item.CrystalA:
		switch {
		case wType == item.WeaponBow:
			value += float64(8*enchant + 16*overenchant)
		case isBigOrDual:
			value += float64(5*enchant + 10*overenchant)
		default:
			value += float64(4*enchant + 8*overenchant)
		}
	case item.CrystalB, item.CrystalC:
		switch {
		case wType == item.WeaponBow:
			value += float64(6*enchant + 12*overenchant)
		case isBigOrDual:
			value += float64(4*enchant + 8*overenchant)
		default:
			value += float64(3*enchant + 6*overenchant)
		}
	case item.CrystalD:
		switch {
		case wType == item.WeaponBow:
			value += float64(4*enchant + 8*overenchant)
		default:
			value += float64(2*enchant + 4*overenchant)
		}
	}

	return value
}
