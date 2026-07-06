package item

import "testing"

func TestNewStackOrEquip_EquippableGranted(t *testing.T) {
	tmpl := &Template{ID: 1146, Kind: KindArmor, Slot: SlotChest}

	inst := NewStackOrEquip(0x10000010, tmpl, 1, true)

	if inst.ObjectID != 0x10000010 || inst.TemplateID != 1146 || inst.Count != 1 {
		t.Fatalf("NewStackOrEquip() = %+v", inst)
	}
	if inst.Location != LocationPaperdoll {
		t.Errorf("Location = %v, want %v", inst.Location, LocationPaperdoll)
	}
	if inst.LocationData != 10 { // CHEST paperdoll position
		t.Errorf("LocationData = %d, want 10", inst.LocationData)
	}
	if inst.ManaLeft != -1 {
		t.Errorf("ManaLeft = %d, want -1", inst.ManaLeft)
	}
}

func TestNewStackOrEquip_NotEquipped(t *testing.T) {
	tmpl := &Template{ID: 10, Kind: KindWeapon, Slot: SlotRHand}

	inst := NewStackOrEquip(0x10000011, tmpl, 1, false)

	if inst.Location != LocationInventory {
		t.Errorf("Location = %v, want %v", inst.Location, LocationInventory)
	}
}

func TestNewStackOrEquip_EtcItemNeverEquips(t *testing.T) {
	tmpl := &Template{ID: 5588, Kind: KindEtcItem, Slot: SlotNone}

	inst := NewStackOrEquip(0x10000012, tmpl, 1, true)

	if inst.Location != LocationInventory {
		t.Errorf("Location = %v, want %v (etc items never equip)", inst.Location, LocationInventory)
	}
}

func TestNewStackOrEquip_TwoHandedSharesRHandPosition(t *testing.T) {
	tmpl := &Template{ID: 2368, Kind: KindWeapon, Slot: SlotLRHand}

	inst := NewStackOrEquip(0x10000013, tmpl, 1, true)

	if inst.Location != LocationPaperdoll {
		t.Errorf("Location = %v, want %v", inst.Location, LocationPaperdoll)
	}
	if inst.LocationData != 7 { // RHAND paperdoll position
		t.Errorf("LocationData = %d, want 7", inst.LocationData)
	}
}
