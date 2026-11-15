package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// randomDamageItems holds a weapon with random damage 10, one with none,
// and a shield.
func randomDamageItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{Type: item.WeaponSword, RandomDamage: 10}},
		{ID: 2, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 3, Kind: item.KindArmor, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
}

// randomDamageCases pairs a right-hand item with the spread an NPC holding
// it rolls: the weapon's random damage, 0 included, or -1 (the level
// fallback) for no weapon. The template's baseRandomDamage never counts.
var randomDamageCases = []struct {
	name      string
	rightHand int
	want      int
}{
	{"weapon", 1, 10},
	{"weapon without random damage", 2, 0},
	{"unarmed", 0, -1},
	{"unknown item", 999, -1},
	{"shield in the right hand", 3, -1},
}

func TestHostileRandomDamageSpreadFollowsWeapon(t *testing.T) {
	for _, tc := range randomDamageCases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := &Template{ID: 20001, Type: "Monster", Level: 70, HPMax: 1000, BaseRandomDamage: 50, RightHand: tc.rightHand}
			h, err := NewHostile(&Instance{ObjectID: 1, Template: tpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
			if err != nil {
				t.Fatal(err)
			}
			h.Attach(Runtime{World: world.New(), Items: randomDamageItems()})
			if got := h.RandomDamageSpread(); got != tc.want {
				t.Fatalf("RandomDamageSpread() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFolkRandomDamageSpreadFollowsWeapon(t *testing.T) {
	for _, tc := range randomDamageCases {
		t.Run(tc.name, func(t *testing.T) {
			inst, err := NewInstance(1, &Template{ID: 31001, TemplateID: 31001, Type: "Folk", Level: 70, HPMax: 100, BaseRandomDamage: 50, RightHand: tc.rightHand})
			if err != nil {
				t.Fatal(err)
			}
			f, err := NewFolk(inst)
			if err != nil {
				t.Fatal(err)
			}
			f.resolveWeapon(randomDamageItems())
			if got := f.RandomDamageSpread(); got != tc.want {
				t.Fatalf("RandomDamageSpread() = %d, want %d", got, tc.want)
			}
		})
	}
}
