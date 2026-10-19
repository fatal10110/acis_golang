package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Reference: BuyListSeed.writeImpl: each row is the type word 4, the seed
// id twice, the amount left, 4 and 0, then the price; no seed writes no
// row count at all.
func TestFrameBuyListSeed(t *testing.T) {
	got := framePayload(t, FrameBuyListSeed(1500, 1, []ManorListing{{ID: 5016, Amount: 7, Price: 30}, {ID: 5012, Amount: 1, Price: 8}}))
	want := appendD(appendD([]byte{OpcodeBuyListSeed}, 1500), 1)
	want = appendH(want, 2)
	want = appendD(appendD(appendD(appendH(want, 4), 5016), 5016), 7)
	want = appendD(appendH(appendH(want, 4), 0), 30)
	want = appendD(appendD(appendD(appendH(want, 4), 5012), 5012), 1)
	want = appendD(appendH(appendH(want, 4), 0), 8)
	checkFrame(t, "FrameBuyListSeed", got, want)

	checkFrame(t, "FrameBuyListSeed(none)", framePayload(t, FrameBuyListSeed(0, 2, nil)), appendD(appendD([]byte{OpcodeBuyListSeed}, 0), 2))
}

// Reference: ExShowSellCropList.writeImpl: a crop the castle buys shows the
// castle, the amount still wanted, the price and reward type; one it does
// not buy shows manor -1 and zeros.
func TestFrameExShowSellCropList(t *testing.T) {
	got := framePayload(t, FrameExShowSellCropList(1, []ManorHeldCrop{
		{ObjectID: 268, CropID: 5073, Count: 12, Seed: darkCoda, Procure: &ManorListing{Amount: 40, Price: 50, Reward: 2}},
		{ObjectID: 269, CropID: 5068, Count: 1},
	}))
	want := appendH([]byte{OpcodeExtended}, 0x21)
	want = appendD(appendD(want, 1), 2)
	want = appendRewards(appendD(appendD(want, 268), 5073), 10, 1864, 1878)
	want = appendD(appendC(appendD(appendD(appendD(want, 1), 40), 50), 2), 12)
	want = appendRewards(appendD(appendD(want, 269), 5068), 0, 0, 0)
	want = appendD(appendC(appendD(appendD(appendD(want, -1), 0), 0), 0), 1)
	checkFrame(t, "FrameExShowSellCropList", got, want)
}

// Reference: ExShowProcureCropDetail.writeImpl.
func TestFrameExShowProcureCropDetail(t *testing.T) {
	got := framePayload(t, FrameExShowProcureCropDetail(5073, []ManorCastleProcure{{CastleID: 1, Amount: 40, Price: 50, Reward: 1}, {CastleID: 2, Amount: 3, Price: 31, Reward: 2}}))
	want := appendD(appendD(appendH([]byte{OpcodeExtended}, 0x22), 5073), 2)
	want = appendC(appendD(appendD(appendD(want, 1), 40), 50), 1)
	want = appendC(appendD(appendD(appendD(want, 2), 3), 31), 2)
	checkFrame(t, "FrameExShowProcureCropDetail", got, want)
}

// Reference: SellListProcure.writeImpl: the item's type words around its
// object and item ids and the amount the castle wants, then 0 and 0.
func TestFrameSellListProcure(t *testing.T) {
	templates := item.NewTable([]*item.Template{{ID: 5073, Kind: item.KindEtcItem, Slot: item.SlotNone}})
	frame, err := FrameSellListProcure(900, []ProcureSale{{Item: &item.Instance{ObjectID: 268, TemplateID: 5073, Count: 12}, Amount: 40}}, templates)
	if err != nil {
		t.Fatal(err)
	}
	want := appendH(appendD(appendD([]byte{OpcodeSellListProcure}, 900), 0), 1)
	want = appendH(want, uint16(item.CategoryMoneyOrEtcItem))
	want = appendD(appendD(appendD(want, 268), 5073), 40)
	want = appendD(appendH(appendH(want, uint16(item.SubCategoryOther)), 0), 0)
	checkFrame(t, "FrameSellListProcure", framePayload(t, frame), want)
}
