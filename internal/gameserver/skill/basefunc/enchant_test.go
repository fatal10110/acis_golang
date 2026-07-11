package basefunc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// fakeEnchantedItem is a fixed EnchantedItem for exercising Enchant's
// crystal-grade/weapon-type matrix without a real inventory system.
type fakeEnchantedItem struct {
	enchant  int
	isWeapon bool
	weapon   item.WeaponType
	crystal  item.CrystalType
}

func (f fakeEnchantedItem) EnchantLevel() int { return f.enchant }
func (f fakeEnchantedItem) Weapon() (item.WeaponType, bool) {
	return f.weapon, f.isWeapon
}
func (f fakeEnchantedItem) Crystal() item.CrystalType { return f.crystal }

// TestEnchantCalc pins the exact per-grade, per-weapon-type constants and
// the +3-past-level-3 overenchant split, cross-checked against the
// reference FuncEnchant switch statement's literal values.
func TestEnchantCalc(t *testing.T) {
	tests := []struct {
		name  string
		s     stat.Stat
		owner fakeEnchantedItem
		want  float64 // delta added to a value of 0
	}{
		{"zero enchant no-op", stat.PowerDefence, fakeEnchantedItem{enchant: 0}, 0},

		{"PDef enchant 2", stat.PowerDefence, fakeEnchantedItem{enchant: 2}, 2},
		{"PDef enchant 5 overenchant", stat.PowerDefence, fakeEnchantedItem{enchant: 5}, 3 + 3*2}, // enchant clamps to 3, over=2
		{"MDef enchant 1", stat.MagicDefence, fakeEnchantedItem{enchant: 1}, 1},

		{"MAtk crystal S enchant 2", stat.MagicAttack, fakeEnchantedItem{enchant: 2, crystal: item.CrystalS}, 4 * 2},
		{"MAtk crystal A enchant 5 overenchant", stat.MagicAttack, fakeEnchantedItem{enchant: 5, crystal: item.CrystalA}, 3*3 + 6*2},
		{"MAtk crystal B enchant 2", stat.MagicAttack, fakeEnchantedItem{enchant: 2, crystal: item.CrystalB}, 3 * 2},
		{"MAtk crystal C enchant 2", stat.MagicAttack, fakeEnchantedItem{enchant: 2, crystal: item.CrystalC}, 3 * 2},
		{"MAtk crystal D enchant 1", stat.MagicAttack, fakeEnchantedItem{enchant: 1, crystal: item.CrystalD}, 2 * 1},
		{"MAtk crystal None no bonus", stat.MagicAttack, fakeEnchantedItem{enchant: 2, crystal: item.CrystalNone}, 0},

		{"PAtk non-weapon no bonus", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: false}, 0},

		{"PAtk S bow enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBow, crystal: item.CrystalS}, 10 * 2},
		{"PAtk S bigsword enchant 5 overenchant", stat.PowerAttack, fakeEnchantedItem{enchant: 5, isWeapon: true, weapon: item.WeaponBigSword, crystal: item.CrystalS}, 6*3 + 12*2},
		{"PAtk S dualfist enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponDualFist, crystal: item.CrystalS}, 6 * 2},
		{"PAtk S dual enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponDual, crystal: item.CrystalS}, 6 * 2},
		{"PAtk S sword (default) enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponSword, crystal: item.CrystalS}, 5 * 2},

		{"PAtk A bow enchant 1", stat.PowerAttack, fakeEnchantedItem{enchant: 1, isWeapon: true, weapon: item.WeaponBow, crystal: item.CrystalA}, 8 * 1},
		{"PAtk A bigblunt enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBigBlunt, crystal: item.CrystalA}, 5 * 2},
		{"PAtk A dagger (default) enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponDagger, crystal: item.CrystalA}, 4 * 2},

		{"PAtk B bow enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBow, crystal: item.CrystalB}, 6 * 2},
		{"PAtk B dual enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponDual, crystal: item.CrystalB}, 4 * 2},
		{"PAtk B sword (default) enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponSword, crystal: item.CrystalB}, 3 * 2},

		{"PAtk C bow enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBow, crystal: item.CrystalC}, 6 * 2},
		{"PAtk C sword (default) enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponSword, crystal: item.CrystalC}, 3 * 2},

		{"PAtk D bow enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBow, crystal: item.CrystalD}, 4 * 2},
		{"PAtk D dagger (default) enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponDagger, crystal: item.CrystalD}, 2 * 2},
		// D grade has no big-weapon distinction in the reference switch: a
		// bigsword falls through to the same default as any other
		// non-bow weapon.
		{"PAtk D bigsword enchant 2", stat.PowerAttack, fakeEnchantedItem{enchant: 2, isWeapon: true, weapon: item.WeaponBigSword, crystal: item.CrystalD}, 2 * 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := NewEnchant(tt.owner, tt.s, 0, nil)
			got := fn.Calc(nil, nil, nil, 0, 0)
			if got != tt.want {
				t.Errorf("Calc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnchantConditionGate(t *testing.T) {
	failing := &fakeCond{result: false}
	fn := NewEnchant(fakeEnchantedItem{enchant: 5}, stat.PowerDefence, 0, failing)
	if got := fn.Calc(nil, nil, nil, 0, 10); got != 10 {
		t.Errorf("Calc() with failing condition = %v, want unchanged 10", got)
	}
	if !failing.invoked {
		t.Error("failing condition was never invoked")
	}
}
