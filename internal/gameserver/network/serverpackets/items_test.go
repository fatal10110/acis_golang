package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ---- from enchant_test.go ----
func TestFrameEnchantResult(t *testing.T) {
	got := framePayload(t, FrameEnchantResult(EnchantResultCancelled))
	want := []byte{OpcodeEnchantResult, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEnchantResult() = %x, want %x", got, want)
	}
}

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

func TestFrameExShowVariationWindows(t *testing.T) {
	got := framePayload(t, FrameExShowVariationMakeWindow())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationMakeWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationMakeWindow() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameExShowVariationCancelWindow())
	want = []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationCancelWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationCancelWindow() = %x, want %x", got, want)
	}
}

// TestFrameExShowQuestInfo pins the quest information window: writeC(0xfe)
// then writeH(0x19), nothing more (ExShowQuestInfo.java).
func TestFrameExShowQuestInfo(t *testing.T) {
	if got, want := framePayload(t, FrameExShowQuestInfo()), []byte{0xfe, 0x19, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowQuestInfo() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmCancelItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmCancelItem(1000, 7575, 0x12345678, 390000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmCancelItem)
	want = appendD(want, 1000)
	want = appendD(want, 7575)
	want = appendD(want, 0x5678)
	want = appendD(want, 0x1234)
	want = appendQ(want, 390000)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmCancelItem() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResultFailed(t *testing.T) {
	got := framePayload(t, FrameExVariationResultFailed())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResultFailed() = %x, want %x", got, want)
	}
}
