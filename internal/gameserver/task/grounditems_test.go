package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

func TestGroundItemsDropLootProtectionLocksThenExpires(t *testing.T) {
	now := time.Unix(0, 0)
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return now })

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1, ProtectOwnerID: 42, ProtectFor: 15 * time.Second})

	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID after drop = %d, want 42 (locked to killer)", got)
	}

	now = now.Add(10 * time.Second)
	g.Tick()
	if got := ground.Instance.Snapshot().OwnerID; got != 42 {
		t.Fatalf("OwnerID before protection deadline = %d, want still 42", got)
	}

	now = now.Add(6 * time.Second) // total 16s > 15s protection window
	g.Tick()
	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID after protection deadline = %d, want 0 (unlocked)", got)
	}
}

func TestGroundItemsLoadClearsPersistedLootProtectionOwner(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{}}})
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) })

	// A snapshot carrying a nonzero OwnerID models a row flushed mid-protection
	// window; Load has no persisted lootExpiresAt to restore alongside it, so
	// Tick's unlock branch (gated on lootExpiresAt) would never fire again if
	// the owner survived the reload, permanently locking the item.
	rows := []item.GroundSnapshot{{
		Instance: item.Instance{ObjectID: 1, TemplateID: 57, Count: 1, OwnerID: 42, Location: item.LocationVoid},
		X:        1, Y: 1, Z: 1,
	}}
	if err := g.Load(rows, templates); err != nil {
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
	g := NewGroundItems(nil, DefaultGroundItemOptions(), func() time.Time { return time.Unix(0, 0) })

	ground := newTestGroundItem(t, 1, 57)
	g.Drop(ground, DropOptions{X: 1, Y: 1, Z: 1})

	if got := ground.Instance.Snapshot().OwnerID; got != 0 {
		t.Fatalf("OwnerID = %d, want 0 for an unprotected drop", got)
	}
}

func TestGroundItemOptionsFromPropertiesAbsentSpecialItemsUsesDefault(t *testing.T) {
	props, err := config.ParseString("")
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := GroundItemOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("GroundItemOptionsFromProperties() error = %v", err)
	}
	want := map[int32]time.Duration{57: 0, 5575: 0, 6673: 0}
	if len(opts.SpecialAutoDestroy) != len(want) {
		t.Fatalf("SpecialAutoDestroy = %v, want %v when AutoDestroySpecialItemTime is absent", opts.SpecialAutoDestroy, want)
	}
	for id, delay := range want {
		if got, ok := opts.SpecialAutoDestroy[id]; !ok || got != delay {
			t.Errorf("SpecialAutoDestroy[%d] = %v, ok=%v, want %v", id, got, ok, delay)
		}
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
