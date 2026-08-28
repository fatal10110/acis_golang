package item

import (
	"sort"
	"testing"
)

// ---- from autoshot_test.go ----
func TestShotItemIDRanges(t *testing.T) {
	tests := []struct {
		name        string
		itemID      int32
		fishingShot bool
		summonShot  bool
	}{
		{"below fishing shots", 6534, false, false},
		{"first fishing shot", 6535, true, false},
		{"last fishing shot", 6540, true, false},
		{"after fishing shots", 6541, false, false},
		{"first summon shot", 6645, false, true},
		{"last summon shot", 6647, false, true},
		{"after summon shots", 6648, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFishingShotID(tt.itemID); got != tt.fishingShot {
				t.Fatalf("IsFishingShotID(%d) = %v, want %v", tt.itemID, got, tt.fishingShot)
			}
			if got := IsSummonShotID(tt.itemID); got != tt.summonShot {
				t.Fatalf("IsSummonShotID(%d) = %v, want %v", tt.itemID, got, tt.summonShot)
			}
		})
	}
}

// ---- from crystal_test.go ----
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

// ---- from drop_test.go ----
func TestParseDropKind(t *testing.T) {
	cases := []struct {
		in   string
		want DropKind
	}{
		{"SPOIL", DropSpoil},
		{"CURRENCY", DropCurrency},
		{"DROP", DropNormal},
		{"HERB", DropHerb},
	}
	for _, c := range cases {
		got, err := ParseDropKind(c.in)
		if err != nil {
			t.Fatalf("ParseDropKind(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseDropKind(%q) = %v, want %v", c.in, got, c.want)
		}
	}

	if _, err := ParseDropKind("BOGUS"); err == nil {
		t.Fatal("ParseDropKind(\"BOGUS\") error = nil, want error")
	}
}

func TestDropRandomAmount(t *testing.T) {
	t.Run("fixed range", func(t *testing.T) {
		d := Drop{Min: 3, Max: 3}
		for i := 0; i < 20; i++ {
			if got := d.RandomAmount(); got != 3 {
				t.Fatalf("RandomAmount() = %d, want 3", got)
			}
		}
	})

	t.Run("within range", func(t *testing.T) {
		d := Drop{Min: 1, Max: 5}
		for i := 0; i < 200; i++ {
			got := d.RandomAmount()
			if got < 1 || got > 5 {
				t.Fatalf("RandomAmount() = %d, want in [1,5]", got)
			}
		}
	})

	t.Run("malformed range does not panic", func(t *testing.T) {
		d := Drop{Min: 5, Max: 1}
		if got := d.RandomAmount(); got != 5 {
			t.Fatalf("RandomAmount() = %d, want Min (5)", got)
		}
	})
}

func TestDropCategoryRollBoundaries(t *testing.T) {
	t.Run("zero category chance never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 0, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(1, 1); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("zero level multiplier never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 100, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(0, 1); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("zero rate never drops", func(t *testing.T) {
		c := DropCategory{Kind: DropNormal, Chance: 100, Drops: []Drop{{ItemID: 1, Min: 1, Max: 1, Chance: 100}}}
		if got := c.Roll(1, 0); got != nil {
			t.Fatalf("Roll() = %v, want nil", got)
		}
	})

	t.Run("guaranteed normal drop picks exactly one entry per rate attempt", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropNormal,
			Chance: 100,
			Drops: []Drop{
				{ItemID: 10, Min: 2, Max: 2, Chance: 50},
				{ItemID: 20, Min: 3, Max: 3, Chance: 50},
			},
		}
		got := c.Roll(1, 3)
		total := int32(0)
		for id, qty := range got {
			if id != 10 && id != 20 {
				t.Fatalf("Roll() produced unexpected item %d", id)
			}
			total += qty
		}
		// 3 attempts, each contributing 2 or 3 units; both bounds are the
		// same 2/3, so the sum must land in [6, 9] and be a multiple
		// achievable by 3 picks of {2,3}.
		if total < 6 || total > 9 {
			t.Fatalf("Roll() total = %d, want in [6,9]", total)
		}
	})

	t.Run("guaranteed spoil drop evaluates every entry independently", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropSpoil,
			Chance: 100,
			Drops: []Drop{
				{ItemID: 10, Min: 1, Max: 1, Chance: 100},
				{ItemID: 20, Min: 1, Max: 1, Chance: 100},
			},
		}
		got := c.Roll(1, 1)
		if got[10] != 1 || got[20] != 1 {
			t.Fatalf("Roll() = %v, want both items at 1 each", got)
		}
	})

	t.Run("fractional rate rolls one extra attempt", func(t *testing.T) {
		c := DropCategory{
			Kind:   DropSpoil,
			Chance: 100,
			Drops:  []Drop{{ItemID: 10, Min: 1, Max: 1, Chance: 100}},
		}
		got := c.Roll(1, 1.5)
		if got[10] != 2 {
			t.Fatalf("Roll() = %v, want item 10 at 2 (two attempts from a 1.5 rate)", got)
		}
	})
}

func TestRatesResolve(t *testing.T) {
	r := Rates{Spoil: 1, Currency: 2, Item: 3, ItemRaid: 4, Herb: 5}

	cases := []struct {
		kind DropKind
		raid bool
		want float64
	}{
		{DropSpoil, false, 1},
		{DropSpoil, true, 1},
		{DropCurrency, false, 2},
		{DropNormal, false, 3},
		{DropNormal, true, 4},
		{DropHerb, false, 5},
	}
	for _, c := range cases {
		if got := r.Resolve(c.kind, c.raid); got != c.want {
			t.Fatalf("Resolve(%v, %v) = %v, want %v", c.kind, c.raid, got, c.want)
		}
	}
}

func TestLevelPenaltyMultiplier(t *testing.T) {
	cases := []struct {
		name                        string
		attackerLevel, monsterLevel int32
		raid, enabled               bool
		want                        float64
	}{
		{"disabled always 1", 80, 10, false, false, 1},
		{"within monster threshold", 14, 10, false, true, 1},
		{"exactly at monster threshold", 15, 10, false, true, 1},
		{"one level past monster threshold", 16, 10, false, true, 1 - 0.18},
		{"within raid threshold", 12, 10, true, true, 1},
		{"one level past raid threshold", 13, 10, true, true, 1 - 0.18},
		{"floored at 0.1", 100, 10, false, true, 0.1},
		{"lower level attacker no penalty", 5, 10, false, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LevelPenaltyMultiplier(c.attackerLevel, c.monsterLevel, c.raid, c.enabled)
			if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("LevelPenaltyMultiplier() = %v, want %v", got, c.want)
			}
		})
	}
}

// ---- from herb_test.go ----
func TestSplitHerbDrop(t *testing.T) {
	t.Run("zero amount yields nothing", func(t *testing.T) {
		if got := SplitHerbDrop(100, 0, false); got != nil {
			t.Fatalf("SplitHerbDrop() = %v, want nil", got)
		}
	})

	t.Run("auto loot always collapses to one stack", func(t *testing.T) {
		got := SplitHerbDrop(100, 5, true)
		want := []HerbPickup{{ItemID: 100, Amount: 1, AutoLoot: true}}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("SplitHerbDrop() = %v, want %v", got, want)
		}
	})

	t.Run("manual pickup yields one pickup per unit", func(t *testing.T) {
		got := SplitHerbDrop(100, 3, false)
		if len(got) != 3 {
			t.Fatalf("SplitHerbDrop() len = %d, want 3", len(got))
		}
		for _, p := range got {
			if p != (HerbPickup{ItemID: 100, Amount: 1}) {
				t.Fatalf("SplitHerbDrop() entry = %v, want {100 1 false}", p)
			}
		}
	})
}

// ---- from instance_persist_test.go ----
func newPersistTestInstance() *Instance {
	return &Instance{
		ObjectID:     0x20000001,
		TemplateID:   57,
		OwnerID:      1,
		Count:        10,
		EnchantLevel: 3,
		Location:     LocationInventory,
		LocationData: 0,
		ManaLeft:     100,
	}
}

func TestInstanceAddCountClampsClientRange(t *testing.T) {
	const maxCount = 1<<31 - 1

	t.Run("saturates additions", func(t *testing.T) {
		inst := &Instance{Count: maxCount - 1}
		if got := inst.AddCount(10); got != maxCount {
			t.Errorf("AddCount() = %d, want %d", got, maxCount)
		}
	})

	t.Run("floors reductions", func(t *testing.T) {
		inst := &Instance{Count: 1}
		if got := inst.AddCount(-2); got != 0 {
			t.Errorf("AddCount() = %d, want 0", got)
		}
	})
}

// TestInstancePersistNotifier pins which mutations schedule a database
// write. A mutation that changes persisted state must report it; one that
// leaves the stored row identical must not, so an unchanged item doesn't
// keep getting rewritten.
func TestInstancePersistNotifier(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Instance)
		mutate func(*Instance)
		want   int
	}{
		{name: "add count", mutate: func(inst *Instance) { inst.AddCount(5) }, want: 1},
		{name: "add no count", mutate: func(inst *Instance) { inst.AddCount(0) }, want: 0},
		{name: "reduce count", mutate: func(inst *Instance) { inst.ReduceCount(4) }, want: 1},
		{name: "reduce more than held", mutate: func(inst *Instance) { inst.ReduceCount(11) }, want: 0},
		{name: "reduce nothing", mutate: func(inst *Instance) { inst.ReduceCount(0) }, want: 0},
		{name: "destroy", mutate: func(inst *Instance) { inst.DestroyState() }, want: 1},
		{
			name:   "set owner location",
			mutate: func(inst *Instance) { inst.SetOwnerLocation(2, LocationWarehouse, 4) },
			want:   1,
		},
		{
			name:   "set same owner location",
			mutate: func(inst *Instance) { inst.SetOwnerLocation(1, LocationInventory, 0) },
			want:   0,
		},
		{name: "set location", mutate: func(inst *Instance) { inst.SetLocation(LocationPaperdoll, 7) }, want: 1},
		{name: "set same location", mutate: func(inst *Instance) { inst.SetLocation(LocationInventory, 0) }, want: 0},
		{name: "set enchant level", mutate: func(inst *Instance) { inst.SetEnchantLevel(4) }, want: 1},
		{name: "set same enchant level", mutate: func(inst *Instance) { inst.SetEnchantLevel(3) }, want: 0},
		{name: "decrease mana", mutate: func(inst *Instance) { inst.DecreaseMana(60) }, want: 1},
		{name: "decrease no mana", mutate: func(inst *Instance) { inst.DecreaseMana(0) }, want: 0},
		{
			name:   "decrease mana on a non-shadow item",
			setup:  func(inst *Instance) { inst.ManaLeft = -1 },
			mutate: func(inst *Instance) { inst.DecreaseMana(60) },
			want:   0,
		},
		{
			name:   "charging a shot is transient",
			mutate: func(inst *Instance) { inst.SetChargedShot(ShotSoul, true) },
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := newPersistTestInstance()
			if tt.setup != nil {
				tt.setup(inst)
			}

			notified := 0
			inst.SetPersistNotifier(func(got *Instance) {
				if got != inst {
					t.Errorf("notifier got instance %p, want %p", got, inst)
				}
				notified++
			})

			tt.mutate(inst)

			if notified != tt.want {
				t.Errorf("notifications = %d, want %d", notified, tt.want)
			}
		})
	}
}

// TestInstancePersistNotifierCleared proves the hook is releasable: an
// instance whose owner detached must stop scheduling writes.
func TestInstancePersistNotifierCleared(t *testing.T) {
	inst := newPersistTestInstance()

	notified := 0
	inst.SetPersistNotifier(func(*Instance) { notified++ })
	inst.AddCount(1)
	if notified != 1 {
		t.Fatalf("notifications before clearing = %d, want 1", notified)
	}

	inst.SetPersistNotifier(nil)
	inst.AddCount(1)
	if notified != 1 {
		t.Errorf("notifications after clearing = %d, want 1", notified)
	}
}

// TestInstancePersistNotifierUnset covers the default: an instance nothing
// persists mutates silently, so domain code needs no persistence layer.
func TestInstancePersistNotifierUnset(t *testing.T) {
	inst := newPersistTestInstance()
	inst.AddCount(1)
	inst.DestroyState()
}

// TestInstanceSnapshotDropsPersistNotifier proves a detached copy doesn't
// inherit the live instance's hook: persisting a snapshot must never
// schedule further writes through the original's owner.
func TestInstanceSnapshotDropsPersistNotifier(t *testing.T) {
	inst := newPersistTestInstance()
	notified := 0
	inst.SetPersistNotifier(func(*Instance) { notified++ })

	clone := inst.Clone()
	clone.AddCount(5)

	if notified != 0 {
		t.Errorf("notifications from clone = %d, want 0", notified)
	}
}

// ---- from instance_predicates_test.go ----
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

// ---- from instance_test.go ----
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

// ---- from item_test.go ----
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

func TestWeaponAndArmorMasksDoNotCollide(t *testing.T) {
	if len(weaponTypeStrings) != weaponTypeCount {
		t.Fatalf("weaponTypeCount = %d, but weaponTypeStrings has %d entries", weaponTypeCount, len(weaponTypeStrings))
	}

	seen := make(map[int32]string)
	for w := WeaponType(0); w < weaponTypeEnd; w++ {
		mask := w.Mask()
		if owner, ok := seen[mask]; ok {
			t.Fatalf("%s and %s share mask %d", owner, w, mask)
		}
		seen[mask] = w.String()
	}
	for a := ArmorType(0); a <= ArmorShield; a++ {
		mask := a.Mask()
		if owner, ok := seen[mask]; ok {
			t.Fatalf("%s and %s share mask %d", owner, a, mask)
		}
		seen[mask] = a.String()
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
		{"quest item", Template{Kind: KindEtcItem, EtcItem: NewEtcItemDetail(EtcItemQuest, "", -1, 0, ActionNone)}, CategoryMoneyOrEtcItem, SubCategoryQuest},
		{"generic etc item", Template{ID: 5588, Kind: KindEtcItem, Slot: SlotNone}, CategoryMoneyOrEtcItem, SubCategoryOther},
	}
	for _, tt := range tests {
		gotCat, gotSub := tt.tmpl.Category()
		if gotCat != tt.wantCat || gotSub != tt.wantSub {
			t.Errorf("%s: Category() = (%d, %d), want (%d, %d)", tt.name, gotCat, gotSub, tt.wantCat, tt.wantSub)
		}
	}
}

// ---- from location_test.go ----
func TestLocationStringRoundTrip(t *testing.T) {
	locations := []Location{
		LocationVoid, LocationInventory, LocationPaperdoll, LocationWarehouse,
		LocationClanWarehouse, LocationPet, LocationPetEquip, LocationFreight,
	}
	for _, l := range locations {
		s := l.String()
		got, err := ParseLocation(s)
		if err != nil {
			t.Errorf("ParseLocation(%q) unexpected error: %v", s, err)
			continue
		}
		if got != l {
			t.Errorf("ParseLocation(%q) = %v, want %v", s, got, l)
		}
	}
}

func TestParseLocation_Unknown(t *testing.T) {
	if _, err := ParseLocation("NOT_A_LOCATION"); err == nil {
		t.Fatal("ParseLocation() with unknown value: want error, got nil")
	}
}

func TestParseLocation_ExactSpelling(t *testing.T) {
	tests := map[string]Location{
		"VOID":      LocationVoid,
		"INVENTORY": LocationInventory,
		"PAPERDOLL": LocationPaperdoll,
		"WAREHOUSE": LocationWarehouse,
		"CLANWH":    LocationClanWarehouse,
		"PET":       LocationPet,
		"PET_EQUIP": LocationPetEquip,
		"FREIGHT":   LocationFreight,
	}
	for s, want := range tests {
		got, err := ParseLocation(s)
		if err != nil {
			t.Errorf("ParseLocation(%q) unexpected error: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLocation(%q) = %v, want %v", s, got, want)
		}
	}
}

// ---- from pet_naming_test.go ----
func TestSetCustomType2PersistsChangedValue(t *testing.T) {
	inst := &Instance{CustomType2: 0}
	var persisted int
	inst.SetPersistNotifier(func(*Instance) { persisted++ })

	if !inst.SetCustomType2(1) {
		t.Fatal("SetCustomType2() = false, want true")
	}
	if got := inst.Snapshot().CustomType2; got != 1 {
		t.Fatalf("CustomType2 = %d, want 1", got)
	}
	if persisted != 1 {
		t.Fatalf("persistence calls = %d, want 1", persisted)
	}
	if inst.SetCustomType2(1) {
		t.Fatal("SetCustomType2(same value) = true, want false")
	}
}

// ---- from reward_test.go ----
func guaranteedCategory(kind DropKind, itemID int32, amount int32) DropCategory {
	return DropCategory{
		Kind:   kind,
		Chance: 100,
		Drops:  []Drop{{ItemID: itemID, Min: amount, Max: amount, Chance: 100}},
	}
}

func TestRollKillRewardSpoilRequiresMarkedPool(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{guaranteedCategory(DropSpoil, 999, 1)}

	t.Run("nil pool skips spoil category", func(t *testing.T) {
		items, herbs := RollKillReward(categories, nil, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil)", items, herbs)
		}
	})

	t.Run("unmarked pool skips spoil category", func(t *testing.T) {
		var pool SpoilPool
		items, herbs := RollKillReward(categories, &pool, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil)", items, herbs)
		}
		if pool.Sweepable() {
			t.Fatal("pool became sweepable despite being unmarked")
		}
	})

	t.Run("marked pool collects the spoil roll", func(t *testing.T) {
		var pool SpoilPool
		pool.Mark(1)
		items, herbs := RollKillReward(categories, &pool, 1, false, rates, false)
		if items != nil || herbs != nil {
			t.Fatalf("RollKillReward() = (%v, %v), want (nil, nil) — spoil goes to the pool", items, herbs)
		}
		got := pool.Sweep()
		if got[999] != 1 {
			t.Fatalf("pool.Sweep() = %v, want {999: 1}", got)
		}
	})
}

func TestRollKillRewardHerbSplitsIntoPickups(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{guaranteedCategory(DropHerb, 500, 3)}

	t.Run("auto loot collapses to one pickup", func(t *testing.T) {
		items, herbs := RollKillReward(categories, nil, 1, false, rates, true)
		if items != nil {
			t.Fatalf("items = %v, want nil", items)
		}
		want := []HerbPickup{{ItemID: 500, Amount: 1, AutoLoot: true}}
		if len(herbs) != 1 || herbs[0] != want[0] {
			t.Fatalf("herbs = %v, want %v", herbs, want)
		}
	})

	t.Run("manual pickup yields one per rolled unit", func(t *testing.T) {
		_, herbs := RollKillReward(categories, nil, 1, false, rates, false)
		if len(herbs) != 3 {
			t.Fatalf("herbs = %v, want 3 entries", herbs)
		}
	})
}

func TestRollKillRewardMergesNormalAndCurrencyIntoItems(t *testing.T) {
	rates := Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}
	categories := []DropCategory{
		guaranteedCategory(DropCurrency, 57, 10),
		guaranteedCategory(DropNormal, 1000, 2),
	}

	items, herbs := RollKillReward(categories, nil, 1, false, rates, false)
	if herbs != nil {
		t.Fatalf("herbs = %v, want nil", herbs)
	}
	want := map[int32]int32{57: 10, 1000: 2}
	if len(items) != len(want) || items[57] != want[57] || items[1000] != want[1000] {
		t.Fatalf("items = %v, want %v", items, want)
	}
}

func TestRollKillRewardUsesRaidRateForNormalDrops(t *testing.T) {
	// A rate below 1 means the category never rolls (Roll's loop condition
	// is float64(i) < rate, so rate 0 never enters the loop). Using Item=0,
	// ItemRaid=1 proves which rate a raid kill actually resolves to.
	rates := Rates{Item: 0, ItemRaid: 1}
	categories := []DropCategory{guaranteedCategory(DropNormal, 1000, 5)}

	items, _ := RollKillReward(categories, nil, 1, false, rates, false)
	if items != nil {
		t.Fatalf("non-raid items = %v, want nil (Item rate is 0)", items)
	}

	items, _ = RollKillReward(categories, nil, 1, true, rates, false)
	if items[1000] != 5 {
		t.Fatalf("raid items = %v, want {1000: 5} (ItemRaid rate is 1)", items)
	}
}

// ---- from shots_test.go ----
func TestInstance_ChargedShot(t *testing.T) {
	inst := &Instance{}

	if inst.ChargedShot(ShotSoul) {
		t.Fatalf("new instance should not be charged")
	}

	inst.SetChargedShot(ShotSoul, true)
	if !inst.ChargedShot(ShotSoul) {
		t.Errorf("ShotSoul should be charged after SetChargedShot(true)")
	}
	if inst.ChargedShot(ShotSpirit) {
		t.Errorf("charging ShotSoul must not also charge ShotSpirit")
	}

	inst.SetChargedShot(ShotSpirit, true)
	inst.SetChargedShot(ShotSoul, false)
	if inst.ChargedShot(ShotSoul) {
		t.Errorf("ShotSoul should be discharged")
	}
	if !inst.ChargedShot(ShotSpirit) {
		t.Errorf("discharging ShotSoul must not discharge ShotSpirit")
	}

	inst.UnchargeAllShots()
	if inst.ChargedShot(ShotSpirit) {
		t.Errorf("UnchargeAllShots should clear every shot")
	}
}

func TestWeaponDetail_EvaluateSoulshot(t *testing.T) {
	weapon := &WeaponDetail{SoulshotCount: 2, ReducedSoulshotChance: 50, ReducedSoulshotCount: 1}

	tests := []struct {
		name           string
		detail         *WeaponDetail
		weaponCrystal  CrystalType
		shotCrystal    CrystalType
		alreadyCharged bool
		roll           int
		wantConsume    int32
		wantOK         bool
	}{
		{"grade match, no reduced roll", weapon, CrystalD, CrystalD, false, 99, 2, true},
		{"grade match, reduced roll hits", weapon, CrystalD, CrystalD, false, 10, 1, true},
		{"grade mismatch", weapon, CrystalD, CrystalC, false, 99, 0, false},
		{"already charged", weapon, CrystalD, CrystalD, true, 99, 0, false},
		{"no soulshot capacity", &WeaponDetail{}, CrystalNone, CrystalNone, false, 99, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consume, ok := tt.detail.EvaluateSoulshot(tt.weaponCrystal, tt.shotCrystal, tt.alreadyCharged, tt.roll)
			if consume != tt.wantConsume || ok != tt.wantOK {
				t.Errorf("EvaluateSoulshot() = (%d, %v), want (%d, %v)", consume, ok, tt.wantConsume, tt.wantOK)
			}
		})
	}
}

func TestWeaponDetail_EvaluateSpiritshot(t *testing.T) {
	weapon := &WeaponDetail{SpiritshotCount: 1}

	if consume, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalS, false); !ok || consume != 1 {
		t.Errorf("EvaluateSpiritshot() = (%d, %v), want (1, true)", consume, ok)
	}
	if _, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalA, false); ok {
		t.Errorf("EvaluateSpiritshot() with mismatched grade should not be ok")
	}
	if _, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalS, true); ok {
		t.Errorf("EvaluateSpiritshot() while already charged should not be ok")
	}
	if _, ok := (&WeaponDetail{}).EvaluateSpiritshot(CrystalNone, CrystalNone, false); ok {
		t.Errorf("EvaluateSpiritshot() with zero capacity should not be ok")
	}
}

// ---- from spoil_test.go ----
func TestSpoilPoolLifecycle(t *testing.T) {
	var pool SpoilPool

	if pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = true before Mark, want false")
	}
	if pool.Sweepable() {
		t.Fatal("Sweepable() = true before any Add, want false")
	}

	pool.Mark(42)
	if !pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = false after Mark, want true")
	}
	if !pool.IsSpoiler(42) {
		t.Fatal("IsSpoiler(42) = false, want true")
	}
	if pool.IsSpoiler(7) {
		t.Fatal("IsSpoiler(7) = true, want false")
	}

	pool.Add(100, 3)
	pool.Add(100, 2)
	pool.Add(200, 1)

	if !pool.Sweepable() {
		t.Fatal("Sweepable() = false after Add, want true")
	}

	got := pool.Sweep()
	want := map[int32]int32{100: 5, 200: 1}
	if len(got) != len(want) || got[100] != want[100] || got[200] != want[200] {
		t.Fatalf("Sweep() = %v, want %v", got, want)
	}

	if pool.Sweepable() {
		t.Fatal("Sweepable() = true after Sweep drained the pool, want false")
	}
	if second := pool.Sweep(); second != nil {
		t.Fatalf("second Sweep() = %v, want nil", second)
	}
	if pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = true after Sweep, want false")
	}

	pool.Reset()
	if pool.IsSpoiled() {
		t.Fatal("IsSpoiled() = true after Reset, want false")
	}
	if pool.Sweepable() {
		t.Fatal("Sweepable() = true after Reset, want false")
	}
}

// ---- from summonitem_test.go ----
