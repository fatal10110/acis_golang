package item

import (
	"sort"
	"testing"
)

func TestSlot_PaperdollIndex(t *testing.T) {
	tests := []struct {
		slot Slot
		want int
	}{
		{SlotUnderwear, 0},
		{SlotLEar, 1},
		{SlotREar, 2},
		{SlotNeck, 3},
		{SlotLFinger, 4},
		{SlotRFinger, 5},
		{SlotHead, 6},
		{SlotRHand, 7},
		{SlotLRHand, 7},
		{SlotLHand, 8},
		{SlotGloves, 9},
		{SlotChest, 10},
		{SlotFullArmor, 10},
		{SlotAllDress, 10},
		{SlotLegs, 11},
		{SlotFeet, 12},
		{SlotBack, 13},
		{SlotFace, 14},
		{SlotHairAll, 14},
		{SlotHair, 15},
	}
	for _, tt := range tests {
		got, ok := tt.slot.PaperdollIndex()
		if !ok {
			t.Errorf("Slot(%d).PaperdollIndex() reported no position, want %d", tt.slot, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("Slot(%d).PaperdollIndex() = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestSlot_PaperdollIndex_PairedSlotsUnresolved(t *testing.T) {
	for _, s := range []Slot{SlotNone, SlotLREar, SlotLRFinger, SlotWolf} {
		if _, ok := s.PaperdollIndex(); ok {
			t.Errorf("Slot(%d).PaperdollIndex() reported a position, want none", s)
		}
	}
}

func TestTable_All(t *testing.T) {
	table := NewTable([]*Template{
		{ID: 30, Name: "c"},
		{ID: 10, Name: "a"},
		{ID: 20, Name: "b"},
	})

	all := table.All()
	if len(all) != table.Len() {
		t.Fatalf("All() returned %d templates, Len() = %d", len(all), table.Len())
	}

	var ids []int32
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i] < ids[j] }) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if ids[0] != 10 || ids[len(ids)-1] != 30 {
		t.Fatalf("All() ids = %v, want [10 20 30]", ids)
	}
}

func TestTemplate_Category(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    Template
		wantCat Category
		wantSub SubCategory
	}{
		{"weapon", Template{Kind: KindWeapon, Slot: SlotRHand}, CategoryWeaponOrJewelry, SubCategoryWeapon},
		{"two-handed weapon", Template{Kind: KindWeapon, Slot: SlotLRHand}, CategoryWeaponOrJewelry, SubCategoryWeapon},
		{"chest armor", Template{Kind: KindArmor, Slot: SlotChest}, CategoryArmor, SubCategoryArmor},
		{"shield", Template{Kind: KindArmor, Slot: SlotLHand}, CategoryArmor, SubCategoryArmor},
		{"necklace", Template{Kind: KindArmor, Slot: SlotNeck}, CategoryWeaponOrJewelry, SubCategoryAccessory},
		{"paired earring", Template{Kind: KindArmor, Slot: SlotLREar}, CategoryWeaponOrJewelry, SubCategoryAccessory},
		{"paired ring", Template{Kind: KindArmor, Slot: SlotLRFinger}, CategoryWeaponOrJewelry, SubCategoryAccessory},
		{"cloak", Template{Kind: KindArmor, Slot: SlotBack}, CategoryWeaponOrJewelry, SubCategoryAccessory},
		{"adena", Template{ID: AdenaID, Kind: KindEtcItem, Slot: SlotNone}, CategoryMoneyOrEtcItem, SubCategoryMoney},
		{"ancient adena", Template{ID: AncientAdenaID, Kind: KindEtcItem, Slot: SlotNone}, CategoryMoneyOrEtcItem, SubCategoryMoney},
		{"generic etc item", Template{ID: 5588, Kind: KindEtcItem, Slot: SlotNone}, CategoryMoneyOrEtcItem, SubCategoryOther},
	}
	for _, tt := range tests {
		gotCat, gotSub := tt.tmpl.Category()
		if gotCat != tt.wantCat || gotSub != tt.wantSub {
			t.Errorf("%s: Category() = (%d, %d), want (%d, %d)", tt.name, gotCat, gotSub, tt.wantCat, tt.wantSub)
		}
	}
}
