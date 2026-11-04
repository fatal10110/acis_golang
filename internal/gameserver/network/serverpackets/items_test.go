package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestWriteItemListNoAuxiliaryAllocation guards against writeItemList
// reintroducing a filtered snapshot slice: with a pre-sized Writer and
// already-locked item.Instances, encoding must not allocate.
func TestWriteItemListNoAuxiliaryAllocation(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
		{ObjectID: 103, TemplateID: 1146, Count: 1, Location: item.LocationWarehouse}, // excluded: not carried
	}

	var w wire.Writer
	w.ResetFrame(256) // pre-size so append growth doesn't allocate during the measured run
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err) // warm up per-item mutex lazy-init before measuring
	}
	warm := append([]byte(nil), w.Bytes()...)

	allocs := testing.AllocsPerRun(100, func() {
		w.ResetFrame(256)
		if err := writeItemList(&w, items, templates, true); err != nil {
			t.Fatalf("writeItemList: %v", err)
		}
	})
	if allocs != 0 {
		t.Errorf("writeItemList allocs/run = %v, want 0 (no auxiliary filter/snapshot slice)", allocs)
	}

	w.ResetFrame(256)
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err)
	}
	if !bytes.Equal(w.Bytes(), warm) {
		t.Errorf("writeItemList output changed across repeated runs:\n got  %x\n want %x", w.Bytes(), warm)
	}
}
