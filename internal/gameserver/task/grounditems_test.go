package task

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// ---- from grounditems_test.go ----

func newTestGroundItem(t *testing.T, id, templateID int32) *grounditem.Item {
	t.Helper()
	tmpl := &item.Template{ID: templateID, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}
	inst := item.Instance{ObjectID: id, TemplateID: templateID, Count: 1, Location: item.LocationVoid}
	ground, err := grounditem.New(inst, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New() error = %v", err)
	}
	return ground
}

// TestGroundItemsDropLootProtectionLiftsAtDeadline: the lock holds until
// the protection runs out and lifts at that instant, with no cleanup sweep.
func TestGroundItemsDropLootProtectionLiftsAtDeadline(t *testing.T) {
	loop := sim.NewInline(time.Unix(0, 0))
	g := NewGroundItems(nil, DefaultGroundItemOptions(), loop.Now, loop.NewQueue("ground-items"))

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1, ProtectOwnerID: 42, ProtectFor: 15 * time.Second})

	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID after drop = %d, want 42 (locked to killer)", got)
	}

	loop.Advance(15*time.Second - time.Nanosecond)
	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID just before the protection deadline = %d, want still 42", got)
	}

	loop.Advance(time.Nanosecond)
	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID at the protection deadline = %d, want 0 (unlocked)", got)
	}
	if snaps := g.Snapshots(nil); len(snaps) != 1 || snaps[0].OwnerID != 0 {
		t.Fatalf("Snapshots() at the protection deadline = %+v, want the drop unowned", snaps)
	}
}

// TestGroundItemsRemoveStopsLootProtectionLift: an item that left the
// ground has nothing left to lift.
func TestGroundItemsRemoveStopsLootProtectionLift(t *testing.T) {
	loop := sim.NewInline(time.Unix(0, 0))
	g := NewGroundItems(nil, DefaultGroundItemOptions(), loop.Now, loop.NewQueue("ground-items"))

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1, ProtectOwnerID: 42, ProtectFor: 15 * time.Second})
	g.Remove(ground)
	if _, armed := loop.NextTimer(); armed {
		t.Fatal("the loot-protection lift is still armed after Remove")
	}
}

func TestGroundItemsLoadClearsPersistedLootProtectionOwner(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}})
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) }, nil)

	// A snapshot carrying a nonzero OwnerID models a row flushed mid-protection
	// window; Load has no persisted lootExpiresAt to restore alongside it, so
	// Tick's unlock branch (gated on lootExpiresAt) would never fire again if
	// the owner survived the reload, permanently locking the item.
	rows := []item.GroundSnapshot{{
		Instance: item.Instance{ObjectID: 1, TemplateID: 57, Count: 1, OwnerID: 42, Location: item.LocationVoid},
		X:        1, Y: 1, Z: 1,
	}}
	if err := g.Load(rows, templates, zerolog.Nop()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	snaps := g.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("Snapshots() len = %d, want 1", len(snaps))
	}
	if snaps[0].OwnerID != 0 {
		t.Fatalf("OwnerID after Load = %d, want 0 (a restart cannot resume a loot-protection lock it never persisted a deadline for)", snaps[0].OwnerID)
	}
}

func TestGroundItemsDropWithoutProtectOwnerLeavesItemUnowned(t *testing.T) {
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) }, nil)

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1})

	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID = %d, want 0 for an unprotected drop", got)
	}
}

// TestGroundItemOptionsFromPropertiesAbsentSpecialItemsGivesNone: with no
// AutoDestroySpecialItemTime no item has a special time, so adena and the
// seal stones expire after AutoDestroyItemTime like any other item.
func TestGroundItemOptionsFromPropertiesAbsentSpecialItemsGivesNone(t *testing.T) {
	props, err := config.ParseString("")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	if len(opts.SpecialAutoDestroy) != 0 {
		t.Fatalf("SpecialAutoDestroy = %v, want none when AutoDestroySpecialItemTime is absent", opts.SpecialAutoDestroy)
	}

	now := time.Unix(0, 0)
	g := NewGroundItems(nil, opts, func() time.Time { return now }, nil)
	g.Drop(newTestGroundItem(t, 1, 57), DropOptions{X: 1, Y: 1, Z: 1})
	now = now.Add(600*time.Second - time.Nanosecond)
	g.Tick()
	if g.Len() != 1 {
		t.Fatal("adena expired before AutoDestroyItemTime")
	}
	now = now.Add(time.Nanosecond)
	g.Tick()
	if g.Len() != 0 {
		t.Fatal("adena still on the ground after AutoDestroyItemTime, want it deleted")
	}
}

// TestGroundItemOptionsFromPropertiesWrapDelays: each time is counted in
// 32-bit milliseconds, so a value past 2147483 seconds wraps, possibly
// negative; a negative delay is already over, and the next sweep deletes
// the drop.
func TestGroundItemOptionsFromPropertiesWrapDelays(t *testing.T) {
	props, err := config.ParseString("AutoDestroyHerbTime=2147484\nAutoDestroyItemTime=3000000\nAutoDestroyEquipableItemTime=5000000\nAutoDestroySpecialItemTime=57-3000000\n")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	for _, tc := range []struct {
		name      string
		got, want time.Duration
	}{
		{"HerbAutoDestroy", opts.HerbAutoDestroy, -2147483296 * time.Millisecond},
		{"ItemAutoDestroy", opts.ItemAutoDestroy, -1294967296 * time.Millisecond},
		{"EquipableAutoDestroy", opts.EquipableAutoDestroy, 705032704 * time.Millisecond},
		{"SpecialAutoDestroy[57]", opts.SpecialAutoDestroy[57], -1294967296 * time.Millisecond},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	g := NewGroundItems(nil, opts, func() time.Time { return time.Unix(0, 0) }, nil)
	g.Drop(newTestGroundItem(t, 1, 1), DropOptions{X: 1, Y: 1, Z: 1})
	g.Tick()
	if g.Len() != 0 {
		t.Fatal("an item with a wrapped negative delay survived the next sweep, want it deleted")
	}
}

func TestGroundItemOptionsFromPropertiesSpecialItemsOverridesDefault(t *testing.T) {
	props, err := config.ParseString("AutoDestroySpecialItemTime=1-10,2-20\n")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	want := map[int32]time.Duration{1: 10 * time.Second, 2: 20 * time.Second}
	if len(opts.SpecialAutoDestroy) != len(want) {
		t.Fatalf("SpecialAutoDestroy = %v, want %v", opts.SpecialAutoDestroy, want)
	}
	for id, delay := range want {
		if got, ok := opts.SpecialAutoDestroy[id]; !ok || got != delay {
			t.Errorf("SpecialAutoDestroy[%d] = %v, ok=%v, want %v", id, got, ok, delay)
		}
	}
}
