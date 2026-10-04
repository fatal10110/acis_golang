package cursedweapon

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
)

// TestReserveHoldsTheWeaponForTheGM: a reserved weapon counts as out, so
// no drop rolls it and a second reservation is refused; Activate makes its
// holder, and StartLife then starts its full life. Unreserve hands back a
// weapon never given, and leaves a held one alone.
func TestReserveHoldsTheWeaponForTheGM(t *testing.T) {
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	m, c := newManager(t, &fakeStore{}, base)

	if !m.Reserve(zariche) || m.Reserve(zariche) {
		t.Fatal("Reserve: want the first to claim Zariche, the second refused")
	}
	if id, ok := m.RollDrop(always(0)); !ok || id != akamanah {
		t.Fatalf("drop = %d %v, want Akamanah: Zariche is reserved", id, ok)
	}
	m.Unreserve(zariche)
	if !m.Reserve(zariche) {
		t.Fatal("Zariche not handed back by Unreserve")
	}

	if m.StartLife(zariche, 7) {
		t.Fatal("StartLife before the weapon is held")
	}
	if _, ok := m.Activate(zariche, Holder{ObjectID: 7, Karma: 100, PKKills: 3}, always(0)); !ok {
		t.Fatal("Activate failed")
	}
	m.Unreserve(zariche)
	c.now = base.Add(time.Minute)
	if m.StartLife(zariche, 8) || !m.StartLife(zariche, 7) {
		t.Fatal("StartLife: want it only for the holder")
	}
	if left := m.TimeLeft(zariche); left != 72*time.Hour {
		t.Fatalf("time left = %v, want 72h", left)
	}
	info := weaponInfo(t, m, zariche)
	if !info.Activated || info.HolderID != 7 || info.HungryMinutes != 24*60 || info.Karma != 100 || info.PKKills != 3 {
		t.Fatalf("info = %+v, want held by 7 with 1440 minutes of hunger", info)
	}
}

// TestEndEndsAWeaponNotOut: End ends a weapon wherever it is, out or not,
// and refuses only an item that is no cursed weapon.
func TestEndEndsAWeaponNotOut(t *testing.T) {
	m, _ := newManager(t, &fakeStore{}, time.Now())
	if end, ok := m.End(akamanah); !ok || end.ItemID != akamanah || end.Held || end.GroundObjectID != 0 {
		t.Fatalf("End = %+v %v, want an end with nothing to release", end, ok)
	}
	if _, ok := m.End(57); ok {
		t.Fatal("End of a common item")
	}
}

// TestReloadEndsEveryWeaponAndTakesTheNewOnes: Reload ends every weapon, in
// the reference's order, then holds the reloaded table's weapons, none out.
func TestReloadEndsEveryWeaponAndTakesTheNewOnes(t *testing.T) {
	m, _ := newManager(t, &fakeStore{}, time.Now())
	m.Activate(zariche, Holder{ObjectID: 7, Karma: 5}, always(0))

	zar, _ := testTable(t).Weapon(zariche)
	zar.Name = "Renamed"
	fresh, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{zar})
	if err != nil {
		t.Fatal(err)
	}
	ends := m.Reload(fresh)
	if len(ends) != 2 || ends[0].ItemID != akamanah || ends[1].ItemID != zariche || !ends[1].Held || ends[1].HolderID != 7 || ends[1].Karma != 5 {
		t.Fatalf("ends = %+v, want Akamanah's, then Zariche's held by 7", ends)
	}
	var names []string
	for _, w := range m.Weapons() {
		if w.Out {
			t.Fatalf("%d still out", w.ItemID)
		}
		names = append(names, w.Name)
	}
	if !slices.Equal(names, []string{"Renamed"}) || m.IsCursed(akamanah) {
		t.Fatalf("weapons = %q, want only the reloaded Zariche", names)
	}
}

func weaponInfo(t *testing.T, m *Manager, itemID int32) Info {
	t.Helper()
	for _, w := range m.Weapons() {
		if w.ItemID == itemID {
			return w
		}
	}
	t.Fatalf("no weapon %d", itemID)
	return Info{}
}
