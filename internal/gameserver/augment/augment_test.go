package augment

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

const (
	testPlayerID int32 = 0x10000001

	swordC     int32 = 100  // C grade sword, 916 crystals
	swordD     int32 = 101  // D grade sword
	heroSword  int32 = 6611 // a hero weapon id, C grade here
	shadowC    int32 = 102  // C grade shadow sword
	fistC      int32 = 103  // C grade fist (WeaponNone)
	rodC       int32 = 104  // C grade fishing rod
	armorC     int32 = 105  // C grade chest, not a weapon
	lifeStone  int32 = 8723 // level 0 life stone, player level 46
	lifeStone2 int32 = 8724 // level 1 life stone, player level 49
	gemstoneD  int32 = 2130
)

func weapon(id int32, grade item.CrystalType, crystals int32, kind item.WeaponType) *item.Template {
	return &item.Template{
		ID: id, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
		Crystal: grade, CrystalCount: crystals,
		Weapon: &item.WeaponDetail{Type: kind},
	}
}

func testTemplates() *item.Table {
	shadow := weapon(shadowC, item.CrystalC, 916, item.WeaponSword)
	shadow.Duration = 60
	return item.NewTable([]*item.Template{
		weapon(swordC, item.CrystalC, 916, item.WeaponSword),
		weapon(swordD, item.CrystalD, 500, item.WeaponSword),
		weapon(heroSword, item.CrystalC, 916, item.WeaponSword),
		shadow,
		weapon(fistC, item.CrystalC, 916, item.WeaponNone),
		weapon(rodC, item.CrystalC, 916, item.WeaponFishingRod),
		{ID: armorC, Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1, Crystal: item.CrystalC, CrystalCount: 916, Armor: &item.ArmorDetail{Type: item.ArmorHeavy}},
		{ID: lifeStone, Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: lifeStone2, Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: gemstoneD, Kind: item.KindEtcItem, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
}

type fixture struct {
	inv    *itemcontainer.Inventory
	nextID int32
}

func newFixture() *fixture {
	return &fixture{inv: itemcontainer.NewPlayerInventory(testPlayerID, testTemplates()), nextID: 0x20000001}
}

func (f *fixture) add(t *testing.T, templateID int32, count int) *item.Instance {
	t.Helper()
	inst := f.inv.AddNew(templateID, count, f.nextID)
	if inst == nil {
		t.Fatalf("AddNew(%d) failed", templateID)
	}
	f.nextID++
	return inst
}

func eligible() State { return State{Level: 80} }

func TestCancelPrice(t *testing.T) {
	cases := []struct {
		name     string
		grade    item.CrystalType
		crystals int32
		enchant  int
		price    int
		ok       bool
	}{
		{"C below 1720", item.CrystalC, 1719, 0, 95000, true},
		{"C at 1720", item.CrystalC, 1720, 0, 150000, true},
		{"C below 2452", item.CrystalC, 2451, 0, 150000, true},
		{"C at 2452", item.CrystalC, 2452, 0, 210000, true},
		{"B below 1746", item.CrystalB, 1745, 0, 240000, true},
		{"B at 1746", item.CrystalB, 1746, 0, 270000, true},
		{"A below 2160", item.CrystalA, 2159, 0, 330000, true},
		{"A at 2160", item.CrystalA, 2160, 0, 390000, true},
		{"A below 2824", item.CrystalA, 2823, 0, 390000, true},
		{"A at 2824", item.CrystalA, 2824, 0, 420000, true},
		{"S", item.CrystalS, 1, 0, 480000, true},
		{"D refused", item.CrystalD, 916, 0, 0, false},
		{"no grade refused", item.CrystalNone, 0, 0, 0, false},
		// The price reads the enchanted crystal count: 916 + 45*(2*10-3)
		// is 1681 at +10, 916 + 45*(2*11-3) is 1771 at +11.
		{"C +10 stays below 1720", item.CrystalC, 916, 10, 95000, true},
		{"C +11 crosses 1720", item.CrystalC, 916, 11, 150000, true},
		// +1..+3 add the plain per-level bonus: 1700 + 45*3 = 1835.
		{"C +3 crosses 1720", item.CrystalC, 1700, 3, 150000, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := weapon(1, tc.grade, tc.crystals, item.WeaponSword)
			price, ok := CancelPrice(tmpl, tc.enchant)
			if price != tc.price || ok != tc.ok {
				t.Fatalf("CancelPrice = (%d, %v), want (%d, %v)", price, ok, tc.price, tc.ok)
			}
		})
	}
	if _, ok := CancelPrice(nil, 0); ok {
		t.Fatal("CancelPrice(nil) ok")
	}
}

func TestConfirmTargetPlayerStateOrder(t *testing.T) {
	cases := []struct {
		name string
		st   State
		want []Message
	}{
		{"operating", State{Level: 80, Operating: true}, []Message{MessageWhileOperating, MessageNotSuitable}},
		{"trading", State{Level: 80, Trading: true}, []Message{MessageWhileTrading, MessageNotSuitable}},
		{"dead", State{Level: 80, Dead: true}, []Message{MessageWhileDead, MessageNotSuitable}},
		{"paralyzed", State{Level: 80, Paralyzed: true}, []Message{MessageWhileParalyzed, MessageNotSuitable}},
		{"fishing", State{Level: 80, Fishing: true}, []Message{MessageWhileFishing, MessageNotSuitable}},
		{"sitting", State{Level: 80, Sitting: true}, []Message{MessageWhileSitting, MessageNotSuitable}},
		{"cursed weapon", State{Level: 80, CursedWeapon: true}, []Message{MessageNotSuitable}},
		// With several flags set, only the first in reference order answers.
		{"operating before trading", State{Level: 80, Operating: true, Trading: true}, []Message{MessageWhileOperating, MessageNotSuitable}},
		{"trading before dead", State{Level: 80, Trading: true, Dead: true}, []Message{MessageWhileTrading, MessageNotSuitable}},
		{"dead before paralyzed", State{Level: 80, Dead: true, Paralyzed: true}, []Message{MessageWhileDead, MessageNotSuitable}},
		{"paralyzed before fishing", State{Level: 80, Paralyzed: true, Fishing: true}, []Message{MessageWhileParalyzed, MessageNotSuitable}},
		{"fishing before sitting", State{Level: 80, Fishing: true, Sitting: true}, []Message{MessageWhileFishing, MessageNotSuitable}},
		{"sitting before cursed weapon", State{Level: 80, Sitting: true, CursedWeapon: true}, []Message{MessageWhileSitting, MessageNotSuitable}},
		{"eligible", eligible(), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			sword := f.add(t, swordC, 1)
			got := (&Service{}).ConfirmTarget(tc.st, testPlayerID, f.inv, sword.ObjectID)
			if !slices.Equal(got.Messages, tc.want) || got.OK != (tc.want == nil) {
				t.Fatalf("ConfirmTarget = %+v, want messages %v ok %v", got, tc.want, tc.want == nil)
			}
		})
	}
}

func TestConfirmTargetItemRefusals(t *testing.T) {
	cases := []struct {
		name     string
		template int32
		augment  bool
		want     []Message
	}{
		{"hero weapon", heroSword, false, []Message{MessageNotSuitable}},
		{"shadow weapon", shadowC, false, []Message{MessageNotSuitable}},
		{"fist", fistC, false, []Message{MessageNotSuitable}},
		{"fishing rod", rodC, false, []Message{MessageNotSuitable}},
		{"below C grade", swordD, false, []Message{MessageNotSuitable}},
		{"not a weapon", armorC, false, []Message{MessageNotSuitable}},
		{"already augmented", swordC, true, []Message{MessageAlreadyAugmented}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			inst := f.add(t, tc.template, 1)
			if tc.augment && !f.inv.SetAugmentation(inst, item.Augmentation{Attributes: 1}) {
				t.Fatal("SetAugmentation failed")
			}
			got := (&Service{}).ConfirmTarget(eligible(), testPlayerID, f.inv, inst.ObjectID)
			if got.OK || !slices.Equal(got.Messages, tc.want) {
				t.Fatalf("ConfirmTarget = %+v, want messages %v", got, tc.want)
			}
		})
	}

	t.Run("augmented weapon while sitting", func(t *testing.T) {
		f := newFixture()
		inst := f.add(t, swordC, 1)
		f.inv.SetAugmentation(inst, item.Augmentation{Attributes: 1})
		got := (&Service{}).ConfirmTarget(State{Level: 80, Sitting: true}, testPlayerID, f.inv, inst.ObjectID)
		want := []Message{MessageWhileSitting, MessageAlreadyAugmented}
		if got.OK || !slices.Equal(got.Messages, want) {
			t.Fatalf("ConfirmTarget = %+v, want messages %v", got, want)
		}
	})

	t.Run("not held answers nothing", func(t *testing.T) {
		f := newFixture()
		if got := (&Service{}).ConfirmTarget(eligible(), testPlayerID, f.inv, 0x7fffffff); got.OK || got.Messages != nil {
			t.Fatalf("ConfirmTarget = %+v, want silence", got)
		}
	})
}

func TestConfirmRefiner(t *testing.T) {
	f := newFixture()
	sword := f.add(t, swordC, 1)
	stone := f.add(t, lifeStone2, 1)
	gem := f.add(t, gemstoneD, 1)

	got := (&Service{}).ConfirmRefiner(State{Level: 48}, testPlayerID, f.inv, sword.ObjectID, stone.ObjectID)
	if want := []Message{MessageLifeStoneLevelTooHigh, MessageNotSuitable}; got.OK || !slices.Equal(got.Messages, want) {
		t.Fatalf("level 48 with a level 49 stone = %+v, want %v", got, want)
	}
	got = (&Service{}).ConfirmRefiner(State{Level: 49}, testPlayerID, f.inv, sword.ObjectID, stone.ObjectID)
	if !got.OK || got.Messages != nil || got.LifeStoneItemID != lifeStone2 || got.GemstoneItemID != gemstoneD || got.GemstoneCount != 20 {
		t.Fatalf("level 49 = %+v, want ok with 20 gemstones %d", got, gemstoneD)
	}
	got = (&Service{}).ConfirmRefiner(State{Level: 80}, testPlayerID, f.inv, sword.ObjectID, gem.ObjectID)
	if want := []Message{MessageNotSuitable}; got.OK || !slices.Equal(got.Messages, want) {
		t.Fatalf("gemstone as life stone = %+v, want %v", got, want)
	}
	// A state refusal answers ahead of the life stone level.
	got = (&Service{}).ConfirmRefiner(State{Level: 1, Dead: true}, testPlayerID, f.inv, sword.ObjectID, stone.ObjectID)
	if want := []Message{MessageWhileDead, MessageNotSuitable}; got.OK || !slices.Equal(got.Messages, want) {
		t.Fatalf("dead = %+v, want %v", got, want)
	}
}

func TestConfirmGemstone(t *testing.T) {
	f := newFixture()
	sword := f.add(t, swordC, 1)
	stone := f.add(t, lifeStone, 1)
	gem := f.add(t, gemstoneD, 19)
	svc := &Service{}

	if got := svc.ConfirmGemstone(eligible(), testPlayerID, f.inv, sword.ObjectID, stone.ObjectID, gem.ObjectID, 20); got.OK ||
		!slices.Equal(got.Messages, []Message{MessageNotSuitable}) {
		t.Fatalf("19 held = %+v, want not suitable", got)
	}
	f.add(t, gemstoneD, 1)
	if got := svc.ConfirmGemstone(eligible(), testPlayerID, f.inv, sword.ObjectID, stone.ObjectID, gem.ObjectID, 19); got.OK ||
		!slices.Equal(got.Messages, []Message{MessageGemstoneQuantityIncorrect}) {
		t.Fatalf("count 19 = %+v, want quantity incorrect", got)
	}
	if got := svc.ConfirmGemstone(eligible(), testPlayerID, f.inv, sword.ObjectID, stone.ObjectID, gem.ObjectID, 20); !got.OK || got.Messages != nil {
		t.Fatalf("count 20 = %+v, want ok", got)
	}
}

func TestConfirmCancel(t *testing.T) {
	f := newFixture()
	plain := f.add(t, swordC, 1)
	if got := ConfirmCancel(testPlayerID, f.inv, plain.ObjectID); got.OK ||
		!slices.Equal(got.Messages, []Message{MessageRemovalNeedsAugmentedItem}) {
		t.Fatalf("unaugmented = %+v, want removal-needs-augmented", got)
	}

	f.inv.SetAugmentation(plain, item.Augmentation{Attributes: 1})
	plain.SetEnchantLevel(11)
	if got := ConfirmCancel(testPlayerID, f.inv, plain.ObjectID); !got.OK || got.Price != 150000 || got.Item != plain {
		t.Fatalf("+11 C sword = %+v, want ok at 150000", got)
	}
	if got := ConfirmCancel(testPlayerID+1, f.inv, plain.ObjectID); got.OK || got.Messages != nil {
		t.Fatalf("other owner = %+v, want silence", got)
	}

	low := f.add(t, swordD, 1)
	low.SetAugmentation(&item.Augmentation{Attributes: 1})
	if got := ConfirmCancel(testPlayerID, f.inv, low.ObjectID); got.OK || got.Messages != nil {
		t.Fatalf("D grade = %+v, want silence", got)
	}
}
