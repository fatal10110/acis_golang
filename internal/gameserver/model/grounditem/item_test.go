package grounditem

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestNew(t *testing.T) {
	tmpl := &item.Template{
		ID:        57,
		Name:      "Adena",
		Kind:      item.KindEtcItem,
		Slot:      item.SlotNone,
		Stackable: true,
		EtcItem:   &item.EtcItemDetail{Type: item.EtcItemNone},
	}
	inst := item.Instance{ObjectID: 0x10000001, TemplateID: 57, Count: 500, EnchantLevel: 3}

	got, err := New(inst, tmpl)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.ObjectID() != inst.ObjectID || got.ItemID() != inst.TemplateID || got.Count() != inst.Count {
		t.Fatalf("ground item = %+v, want instance %+v", got, inst)
	}
	if !got.Stackable() {
		t.Fatal("Stackable() = false, want true")
	}
	if got.Herb() {
		t.Fatal("Herb() = true for Adena")
	}

	got.SetDestroyProtected(true)
	if !got.DestroyProtected() {
		t.Fatal("DestroyProtected() = false after SetDestroyProtected(true)")
	}
}

func TestNewRejectsMismatchedTemplate(t *testing.T) {
	_, err := New(item.Instance{ObjectID: 1, TemplateID: 57}, &item.Template{ID: 10})
	if err == nil {
		t.Fatal("New() error = nil, want mismatch error")
	}
}

func TestSnapshotRecordsPositionAndRemainingTime(t *testing.T) {
	tmpl := &item.Template{ID: 10, Kind: item.KindWeapon, Slot: item.SlotRHand}
	got, err := New(item.Instance{ObjectID: 1, TemplateID: 10, Count: 1, EnchantLevel: 7}, tmpl)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	world.New().Spawn(got, 100, 200, -50, 0)

	snap := got.Snapshot(12_345)
	if snap.ObjectID != 1 || snap.TemplateID != 10 || snap.Count != 1 || snap.EnchantLevel != 7 {
		t.Fatalf("snapshot instance fields = %+v", snap)
	}
	if snap.X != 100 || snap.Y != 200 || snap.Z != -50 || snap.TimeLeftMillis != 12_345 {
		t.Fatalf("snapshot location/time = %+v", snap)
	}
}
