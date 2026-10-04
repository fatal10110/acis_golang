package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

type autoShotSummon struct{ ss, sps int }

func (s autoShotSummon) SSCount() int  { return s.ss }
func (s autoShotSummon) SPSCount() int { return s.sps }

func TestCharacterToggleAutoSoulShotAppliesItemRules(t *testing.T) {
	c := &Character{}

	// No weapon held: a weapon shot still turns on, as a grade mismatch.
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 1463, Enabled: true, Held: true, ItemCount: 1}); status != AutoSoulShotGradeMismatch {
		t.Fatalf("ToggleAutoSoulShot regular status = %v, want grade mismatch", status)
	}
	if !c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot was not enabled")
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 1463, Held: true, ItemCount: 1}); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot disable status = %v, want toggled", status)
	}
	if c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot remained enabled after disable")
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 6535, Enabled: true, Held: true, ItemCount: 1}); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot fishing status = %v, want noop", status)
	}
	if c.AutoSoulShotEnabled(6535) {
		t.Fatal("fishing shot was enabled")
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 6645, Enabled: true, Held: true, ItemCount: 1}); status != AutoSoulShotNeedsSummon {
		t.Fatalf("ToggleAutoSoulShot summon status = %v, want needs summon", status)
	}
	if c.AutoSoulShotEnabled(6645) {
		t.Fatal("summon shot was enabled without a summon")
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 1463, Enabled: true, ItemCount: 5, Summon: autoShotSummon{1, 1}}); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot missing item status = %v, want noop", status)
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 6645, Enabled: true, Held: true, ItemCount: 3, Summon: autoShotSummon{ss: 3, sps: 9}}); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot summon shot status = %v, want toggled", status)
	}
	if !c.AutoSoulShotEnabled(6645) {
		t.Fatal("summon shot covering one charge was not enabled")
	}
}

// TestAutoShotEnableGateOrder pins the enable-side rejections of
// RequestAutoSoulShot.java:34-106 in their order: no summon before the
// Olympiad blessed-servitor-shot reject before the per-charge count, the
// count read from the summon's soulshot or spiritshot need by item id, and
// the graded blessed spiritshots refused in Olympiad. Plain weapon shots
// pass in Olympiad.
func TestAutoShotEnableGateOrder(t *testing.T) {
	pet := autoShotSummon{ss: 2, sps: 4}
	cases := []struct {
		name     string
		req      AutoSoulShotRequest
		olympiad bool
		want     AutoSoulShotStatus
	}{
		{"fishing shot", AutoSoulShotRequest{ItemID: 6537, Held: true, ItemCount: 10}, true, AutoSoulShotNoop},
		{"servitor bss no summon in olympiad", AutoSoulShotRequest{ItemID: 6647, Held: true, ItemCount: 10}, true, AutoSoulShotNeedsSummon},
		{"servitor bss in olympiad", AutoSoulShotRequest{ItemID: 6647, Held: true, ItemCount: 10, Summon: pet}, true, AutoSoulShotOlympiadBlocked},
		{"servitor bss short in olympiad", AutoSoulShotRequest{ItemID: 6647, Held: true, ItemCount: 1, Summon: pet}, true, AutoSoulShotOlympiadBlocked},
		{"servitor bss outside olympiad", AutoSoulShotRequest{ItemID: 6647, Held: true, ItemCount: 4, Summon: pet}, false, AutoSoulShotToggled},
		{"servitor bss short", AutoSoulShotRequest{ItemID: 6647, Held: true, ItemCount: 3, Summon: pet}, false, AutoSoulShotNotEnoughForPet},
		{"servitor sps in olympiad", AutoSoulShotRequest{ItemID: 6646, Held: true, ItemCount: 4, Summon: pet}, true, AutoSoulShotToggled},
		{"servitor sps short", AutoSoulShotRequest{ItemID: 6646, Held: true, ItemCount: 3, Summon: pet}, false, AutoSoulShotNotEnoughForPet},
		{"servitor ss uses soulshot need", AutoSoulShotRequest{ItemID: 6645, Held: true, ItemCount: 2, Summon: pet}, false, AutoSoulShotToggled},
		{"servitor ss short", AutoSoulShotRequest{ItemID: 6645, Held: true, ItemCount: 1, Summon: pet}, false, AutoSoulShotNotEnoughForPet},
		{"weapon bss no-grade in olympiad", AutoSoulShotRequest{ItemID: 3947, Held: true, ItemCount: 1}, true, AutoSoulShotOlympiadBlocked},
		{"weapon bss s-grade in olympiad", AutoSoulShotRequest{ItemID: 3952, Held: true, ItemCount: 1}, true, AutoSoulShotOlympiadBlocked},
		{"weapon bss outside olympiad", AutoSoulShotRequest{ItemID: 3952, Held: true, ItemCount: 1}, false, AutoSoulShotToggled},
		{"weapon sps in olympiad", AutoSoulShotRequest{ItemID: 2514, Held: true, ItemCount: 1}, true, AutoSoulShotToggled},
		{"weapon ss in olympiad", AutoSoulShotRequest{ItemID: 1467, Held: true, ItemCount: 1}, true, AutoSoulShotToggled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Enabled = true
			if got := autoShotEnableGate(tc.req, tc.olympiad); got != tc.want {
				t.Fatalf("autoShotEnableGate = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestToggleAutoSoulShotGradeMatchesActiveWeapon pins the grade test of
// RequestAutoSoulShot.java:92-100: an enabled weapon shot is a match only
// when a weapon is held and its crystal grade is the shot's. A mismatch
// still turns auto use on.
func TestToggleAutoSoulShotGradeMatchesActiveWeapon(t *testing.T) {
	items := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalC, Weapon: &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1}},
		{ID: 1464, Kind: item.KindEtcItem, Crystal: item.CrystalC, EtcItem: &item.EtcItemDetail{}},
		{ID: 1465, Kind: item.KindEtcItem, Crystal: item.CrystalB, EtcItem: &item.EtcItemDetail{}},
	})
	sword := &item.Instance{ObjectID: 101, TemplateID: 1, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand, ManaLeft: -1}
	c := liveCharacter(1, combatTemplate(), items, sword)

	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 1464, Enabled: true, Held: true, ItemCount: 5}); status != AutoSoulShotToggled {
		t.Fatalf("C-grade shot with C-grade sword = %v, want toggled", status)
	}
	if status := c.ToggleAutoSoulShot(AutoSoulShotRequest{ItemID: 1465, Enabled: true, Held: true, ItemCount: 5}); status != AutoSoulShotGradeMismatch {
		t.Fatalf("B-grade shot with C-grade sword = %v, want grade mismatch", status)
	}
	if !c.AutoSoulShotEnabled(1464) || !c.AutoSoulShotEnabled(1465) {
		t.Fatal("matching and mismatched shots must both be on")
	}
}
