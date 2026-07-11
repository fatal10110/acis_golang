package item

import "testing"

func TestInstance_AugmentedGatesTransactionPredicates(t *testing.T) {
	tmpl := &Template{Dropable: true, Tradable: true, Sellable: true, Destroyable: true, Depositable: true, EtcItem: &EtcItemDetail{}}
	inst := &Instance{}

	if !inst.Dropable(tmpl) || !inst.Tradable(tmpl) || !inst.Sellable(tmpl) {
		t.Fatalf("an unaugmented instance should follow the template's flags")
	}

	inst.Augmentation = &Augmentation{Attributes: 1}
	if !inst.Augmented() {
		t.Fatalf("Augmented() should report true once an augmentation is set")
	}
	if inst.Dropable(tmpl) || inst.Tradable(tmpl) || inst.Sellable(tmpl) {
		t.Errorf("an augmented instance must never be dropable, tradable, or sellable regardless of template flags")
	}
	// Destroyable is gated by quest-item status, not augmentation.
	if !inst.Destroyable(tmpl) {
		t.Errorf("augmentation should not affect Destroyable")
	}
}

func TestInstance_QuestItemGatesDestroyable(t *testing.T) {
	quest := &Template{Destroyable: true, EtcItem: &EtcItemDetail{Type: EtcItemQuest}}
	regular := &Template{Destroyable: true, EtcItem: &EtcItemDetail{Type: EtcItemMaterial}}
	inst := &Instance{}

	if inst.Destroyable(quest) {
		t.Errorf("a quest item must never be destroyable regardless of the template's Destroyable flag")
	}
	if !inst.Destroyable(regular) {
		t.Errorf("a non-quest item should follow the template's Destroyable flag")
	}
}

func TestInstance_Depositable(t *testing.T) {
	tmpl := &Template{Depositable: true, Tradable: true, Duration: -1, EtcItem: &EtcItemDetail{}}
	shadow := &Template{Depositable: true, Tradable: true, Duration: 3600, EtcItem: &EtcItemDetail{}}

	equipped := &Instance{Location: LocationPaperdoll}
	if equipped.Depositable(tmpl, true) {
		t.Errorf("an equipped item must never be depositable")
	}

	unequipped := &Instance{Location: LocationInventory}
	if !unequipped.Depositable(tmpl, true) {
		t.Errorf("a private warehouse should accept any otherwise-depositable item")
	}
	if !unequipped.Depositable(tmpl, false) {
		t.Errorf("a public warehouse should accept a tradable, non-shadow item")
	}

	shadowInst := &Instance{Location: LocationInventory}
	if !shadowInst.Depositable(shadow, true) {
		t.Errorf("a private warehouse should still accept a shadow item")
	}
	if shadowInst.Depositable(shadow, false) {
		t.Errorf("a public warehouse must reject a shadow item")
	}
}

func TestInstance_ShadowItemAndDisplayedManaLeft(t *testing.T) {
	shadow := &Template{Duration: 60}
	regular := &Template{Duration: -1}
	inst := &Instance{ManaLeft: 125}

	if !inst.ShadowItem(shadow) {
		t.Errorf("a template with a non-negative duration should be a shadow item")
	}
	if inst.ShadowItem(regular) {
		t.Errorf("a template with duration -1 should not be a shadow item")
	}
	if got := inst.DisplayedManaLeft(shadow); got != 2 {
		t.Errorf("DisplayedManaLeft() = %d, want 2 (125s rounded down to whole minutes)", got)
	}
	if got := inst.DisplayedManaLeft(regular); got != -1 {
		t.Errorf("DisplayedManaLeft() on a non-shadow item = %d, want -1", got)
	}
}

func TestInstance_DecreaseMana(t *testing.T) {
	inst := &Instance{ManaLeft: 5}
	inst.DecreaseMana(2)
	if inst.ManaLeft != 3 {
		t.Errorf("ManaLeft = %d, want 3", inst.ManaLeft)
	}
	inst.DecreaseMana(10)
	if inst.ManaLeft != 0 {
		t.Errorf("DecreaseMana should floor at zero, got %d", inst.ManaLeft)
	}

	inst.ManaLeft = -1
	inst.DecreaseMana(1)
	if inst.ManaLeft != -1 {
		t.Errorf("DecreaseMana on an untracked item changed ManaLeft to %d, want -1", inst.ManaLeft)
	}

	inst.ManaLeft = 5
	inst.DecreaseMana(0)
	inst.DecreaseMana(-3)
	if inst.ManaLeft != 5 {
		t.Errorf("DecreaseMana with non-positive amounts changed ManaLeft to %d, want 5", inst.ManaLeft)
	}
}

func TestInstance_Equipped(t *testing.T) {
	tests := []struct {
		loc  Location
		want bool
	}{
		{LocationInventory, false},
		{LocationPaperdoll, true},
		{LocationPetEquip, true},
		{LocationWarehouse, false},
	}
	for _, tt := range tests {
		inst := &Instance{Location: tt.loc}
		if got := inst.Equipped(); got != tt.want {
			t.Errorf("Equipped() with Location=%v = %v, want %v", tt.loc, got, tt.want)
		}
	}
}
