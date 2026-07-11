package item

import "testing"

func TestTemplate_CrystalCountAt(t *testing.T) {
	weapon := &Template{Kind: KindWeapon, Crystal: CrystalS, CrystalCount: 180, Weapon: &WeaponDetail{}}
	armor := &Template{Kind: KindArmor, Slot: SlotChest, Crystal: CrystalS, CrystalCount: 90, Armor: &ArmorDetail{}}
	accessory := &Template{Kind: KindArmor, Slot: SlotNeck, Crystal: CrystalA, CrystalCount: 20, Armor: &ArmorDetail{}}
	etc := &Template{Kind: KindEtcItem, Crystal: CrystalD, CrystalCount: 5, EtcItem: &EtcItemDetail{}}

	tests := []struct {
		name         string
		tmpl         *Template
		enchantLevel int
		want         int32
	}{
		{"weapon unenchanted", weapon, 0, 180},
		{"weapon +2 uses weapon bonus * level", weapon, 2, 180 + 250*2},
		{"weapon +5 uses weapon bonus * (2*level-3)", weapon, 5, 180 + 250*(2*5-3)},
		{"armor +2 uses armor bonus * level", armor, 2, 90 + 25*2},
		{"armor +5 uses armor bonus * (3*level-6)", armor, 5, 90 + 25*(3*5-6)},
		{"accessory follows armor formula", accessory, 2, 20 + 19*2},
		{"etc item never gets an enchant bonus", etc, 5, 5},
		{"negative enchant level is treated as unenchanted", weapon, -1, 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tmpl.CrystalCountAt(tt.enchantLevel); got != tt.want {
				t.Errorf("CrystalCountAt(%d) = %d, want %d", tt.enchantLevel, got, tt.want)
			}
		})
	}
}

func TestTemplate_CrystalReward(t *testing.T) {
	crystallizable := &Template{Kind: KindWeapon, Crystal: CrystalS, CrystalCount: 180, Weapon: &WeaponDetail{}}
	notCrystallizable := &Template{Kind: KindEtcItem, Crystal: CrystalNone, CrystalCount: 0, EtcItem: &EtcItemDetail{}}
	zeroCount := &Template{Kind: KindWeapon, Crystal: CrystalD, CrystalCount: 0, Weapon: &WeaponDetail{}}

	if id, count, ok := crystallizable.CrystalReward(0); !ok || id != 1462 || count != 180 {
		t.Errorf("CrystalReward(0) = (%d, %d, %v), want (1462, 180, true)", id, count, ok)
	}
	if _, _, ok := notCrystallizable.CrystalReward(0); ok {
		t.Errorf("CrystalReward() on a NONE-crystal template should not be ok")
	}
	if _, _, ok := zeroCount.CrystalReward(0); ok {
		t.Errorf("CrystalReward() on a zero-CrystalCount template should not be ok")
	}
}

func TestCanCrystallize(t *testing.T) {
	tests := []struct {
		crystal    CrystalType
		skillLevel int
		want       bool
	}{
		{CrystalD, 0, false},
		{CrystalD, 1, true},
		{CrystalC, 1, false},
		{CrystalC, 2, true},
		{CrystalB, 2, false},
		{CrystalB, 3, true},
		{CrystalA, 3, false},
		{CrystalA, 4, true},
		{CrystalS, 4, false},
		{CrystalS, 5, true},
		{CrystalNone, 1, true},
	}
	for _, tt := range tests {
		if got := CanCrystallize(tt.crystal, tt.skillLevel); got != tt.want {
			t.Errorf("CanCrystallize(%v, %d) = %v, want %v", tt.crystal, tt.skillLevel, got, tt.want)
		}
	}
}
