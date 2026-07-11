package item

// crystalTypeData holds the fixed, client-defined constants associated
// with a CrystalType: none of this comes from shipped XML data, so it's
// tabulated directly rather than parsed.
type crystalTypeData struct {
	itemID             int32
	enchantBonusArmor  int32
	enchantBonusWeapon int32
}

var crystalTypeTable = map[CrystalType]crystalTypeData{
	CrystalNone: {itemID: 0, enchantBonusArmor: 0, enchantBonusWeapon: 0},
	CrystalD:    {itemID: 1458, enchantBonusArmor: 11, enchantBonusWeapon: 90},
	CrystalC:    {itemID: 1459, enchantBonusArmor: 6, enchantBonusWeapon: 45},
	CrystalB:    {itemID: 1460, enchantBonusArmor: 11, enchantBonusWeapon: 67},
	CrystalA:    {itemID: 1461, enchantBonusArmor: 19, enchantBonusWeapon: 144},
	CrystalS:    {itemID: 1462, enchantBonusArmor: 25, enchantBonusWeapon: 250},
}

// ItemID returns the crystal item produced by crystallizing an item of
// grade c, or 0 for CrystalNone.
func (c CrystalType) ItemID() int32 {
	return crystalTypeTable[c].itemID
}

// EnchantBonusArmor returns the per-enchant-level crystal count bonus an
// armor or accessory of grade c earns when crystallized.
func (c CrystalType) EnchantBonusArmor() int32 {
	return crystalTypeTable[c].enchantBonusArmor
}

// EnchantBonusWeapon returns the per-enchant-level crystal count bonus a
// weapon of grade c earns when crystallized.
func (c CrystalType) EnchantBonusWeapon() int32 {
	return crystalTypeTable[c].enchantBonusWeapon
}

// Crystallizable reports whether t can be crystallized at all.
func (t *Template) Crystallizable() bool {
	return t.Crystal != CrystalNone && t.CrystalCount > 0
}

// CrystalCountAt returns the number of crystals crystallizing t yields for
// an instance enchanted to enchantLevel. Weapons and armor/accessories earn
// a per-level bonus above +3 and above +0 respectively; every other
// category (and any enchant level at or below 0) yields the template's
// base CrystalCount unchanged.
func (t *Template) CrystalCountAt(enchantLevel int) int32 {
	_, sub := t.Category()

	switch {
	case enchantLevel > 3:
		switch sub {
		case SubCategoryArmor, SubCategoryAccessory:
			return t.CrystalCount + t.Crystal.EnchantBonusArmor()*int32(3*enchantLevel-6)
		case SubCategoryWeapon:
			return t.CrystalCount + t.Crystal.EnchantBonusWeapon()*int32(2*enchantLevel-3)
		default:
			return t.CrystalCount
		}
	case enchantLevel > 0:
		switch sub {
		case SubCategoryArmor, SubCategoryAccessory:
			return t.CrystalCount + t.Crystal.EnchantBonusArmor()*int32(enchantLevel)
		case SubCategoryWeapon:
			return t.CrystalCount + t.Crystal.EnchantBonusWeapon()*int32(enchantLevel)
		default:
			return t.CrystalCount
		}
	default:
		return t.CrystalCount
	}
}

// CrystalReward returns the crystal item id and count crystallizing an
// instance of t enchanted to enchantLevel produces. ok is false when t
// can't be crystallized at all (eligibility is judged on the template's
// base CrystalCount, matching the crystallize request's own check, even
// though the yielded count is enchant-adjusted). The caller is still
// responsible for the instance-level gates this method doesn't know about
// (hero items and shadow items can never be crystallized regardless of
// template).
func (t *Template) CrystalReward(enchantLevel int) (itemID int32, count int32, ok bool) {
	if !t.Crystallizable() || t.CrystalCount <= 0 || t.Crystal == CrystalNone {
		return 0, 0, false
	}
	return t.Crystal.ItemID(), t.CrystalCountAt(enchantLevel), true
}

// CanCrystallize reports whether a player whose Crystallize skill sits at
// skillLevel may crystallize an item of crystalType. skillLevel <= 0 means
// the player doesn't own the skill at all, which always denies.
func CanCrystallize(crystalType CrystalType, skillLevel int) bool {
	if skillLevel <= 0 {
		return false
	}
	switch crystalType {
	case CrystalC:
		return skillLevel > 1
	case CrystalB:
		return skillLevel > 2
	case CrystalA:
		return skillLevel > 3
	case CrystalS:
		return skillLevel > 4
	default:
		return true
	}
}
