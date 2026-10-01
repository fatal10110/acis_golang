package clientpackets

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from item_ops_test.go ----
func TestDecodeRequestDropItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestDropItem,
		0xf4, 0x01, 0x00, 0x00,
		0x28, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}

	got, err := DecodeRequestDropItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDropItem: %v", err)
	}
	want := RequestDropItem{ObjectID: 500, Count: 40, X: 46160, Y: 41237, Z: -3534}
	if got != want {
		t.Fatalf("DecodeRequestDropItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestDestroyItem(t *testing.T) {
	payload := []byte{OpcodeRequestDestroyItem, 0xf5, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}

	got, err := DecodeRequestDestroyItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestDestroyItem: %v", err)
	}
	want := RequestDestroyItem{ObjectID: 501, Count: 2}
	if got != want {
		t.Fatalf("DecodeRequestDestroyItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestCrystallizeItem(t *testing.T) {
	payload := []byte{OpcodeRequestCrystallizeItem, 0xf6, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeRequestCrystallizeItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestCrystallizeItem: %v", err)
	}
	want := RequestCrystallizeItem{ObjectID: 502, Count: 1}
	if got != want {
		t.Fatalf("DecodeRequestCrystallizeItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestEnchantItem(t *testing.T) {
	payload := []byte{OpcodeRequestEnchantItem, 0xf7, 0x01, 0x00, 0x00}

	got, err := DecodeRequestEnchantItem(payload)
	if err != nil {
		t.Fatalf("DecodeRequestEnchantItem: %v", err)
	}
	want := RequestEnchantItem{ObjectID: 503}
	if got != want {
		t.Fatalf("DecodeRequestEnchantItem = %+v, want %+v", got, want)
	}
}

func TestDecodePetItemRequests(t *testing.T) {
	use, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 0x21, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetUseItem: %v", err)
	}
	if use != (RequestPetUseItem{ObjectID: 801}) {
		t.Fatalf("DecodeRequestPetUseItem = %+v, want ObjectID 801", use)
	}

	give, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 0x22, 0x03, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestGiveItemToPet: %v", err)
	}
	if give != (RequestGiveItemToPet{ObjectID: 802, Count: 5}) {
		t.Fatalf("DecodeRequestGiveItemToPet = %+v, want ObjectID 802 Count 5", give)
	}

	take, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 0x23, 0x03, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff})
	if err != nil {
		t.Fatalf("DecodeRequestGetItemFromPet: %v", err)
	}
	if take != (RequestGetItemFromPet{ObjectID: 803, Count: 6, Unknown: -1}) {
		t.Fatalf("DecodeRequestGetItemFromPet = %+v, want ObjectID 803 Count 6 Unknown -1", take)
	}

	pickup, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 0x24, 0x03, 0x00, 0x00})
	if err != nil {
		t.Fatalf("DecodeRequestPetGetItem: %v", err)
	}
	if pickup != (RequestPetGetItem{ObjectID: 804}) {
		t.Fatalf("DecodeRequestPetGetItem = %+v, want ObjectID 804", pickup)
	}
}

func TestDecodeSendTimeCheck(t *testing.T) {
	payload := []byte{OpcodeSendTimeCheck, 0x11, 0x00, 0x00, 0x00, 0x22, 0x00, 0x00, 0x00}

	got, err := DecodeSendTimeCheck(payload)
	if err != nil {
		t.Fatalf("DecodeSendTimeCheck: %v", err)
	}
	want := SendTimeCheck{RequestID: 17, ResponseID: 34}
	if got != want {
		t.Fatalf("DecodeSendTimeCheck = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestAutoSoulShot(t *testing.T) {
	payload := []byte{
		OpcodeExtended,
		0x05, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestAutoSoulShot(payload)
	if err != nil {
		t.Fatalf("DecodeRequestAutoSoulShot: %v", err)
	}
	want := RequestAutoSoulShot{ItemID: 1463, Type: 1}
	if got != want {
		t.Fatalf("DecodeRequestAutoSoulShot = %+v, want %+v", got, want)
	}
}

func TestDecodeItemOpsShort(t *testing.T) {
	if _, err := DecodeRequestDropItem([]byte{OpcodeRequestDropItem, 1}); err == nil {
		t.Fatal("DecodeRequestDropItem: want error on short payload")
	}
	if _, err := DecodeRequestDestroyItem([]byte{OpcodeRequestDestroyItem, 1}); err == nil {
		t.Fatal("DecodeRequestDestroyItem: want error on short payload")
	}
	if _, err := DecodeRequestCrystallizeItem([]byte{OpcodeRequestCrystallizeItem, 1}); err == nil {
		t.Fatal("DecodeRequestCrystallizeItem: want error on short payload")
	}
	if _, err := DecodeRequestEnchantItem([]byte{OpcodeRequestEnchantItem, 1}); err == nil {
		t.Fatal("DecodeRequestEnchantItem: want error on short payload")
	}
	if _, err := DecodeRequestPetUseItem([]byte{OpcodeRequestPetUseItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetUseItem: want error on short payload")
	}
	if _, err := DecodeRequestGiveItemToPet([]byte{OpcodeRequestGiveItemToPet, 1}); err == nil {
		t.Fatal("DecodeRequestGiveItemToPet: want error on short payload")
	}
	if _, err := DecodeRequestGetItemFromPet([]byte{OpcodeRequestGetItemFromPet, 1}); err == nil {
		t.Fatal("DecodeRequestGetItemFromPet: want error on short payload")
	}
	if _, err := DecodeRequestPetGetItem([]byte{OpcodeRequestPetGetItem, 1}); err == nil {
		t.Fatal("DecodeRequestPetGetItem: want error on short payload")
	}
	if _, err := DecodeSendTimeCheck([]byte{OpcodeSendTimeCheck, 1}); err == nil {
		t.Fatal("DecodeSendTimeCheck: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x05, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on short payload")
	}
	if _, err := DecodeRequestAutoSoulShot([]byte{OpcodeExtended, 0x08, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestAutoSoulShot: want error on wrong extended opcode")
	}
}

// ---- from pet_name_test.go ----
func TestDecodeRequestChangePetName(t *testing.T) {
	w := wire.NewPacketWriter(OpcodeRequestChangePetName)
	w.WriteString("Rex")

	got, err := DecodeRequestChangePetName(w.Bytes())
	if err != nil {
		t.Fatalf("DecodeRequestChangePetName: %v", err)
	}
	if got.Name != "Rex" {
		t.Fatalf("Name = %q, want Rex", got.Name)
	}
}

func TestDecodeRequestChangePetNameShort(t *testing.T) {
	if _, err := DecodeRequestChangePetName([]byte{OpcodeRequestChangePetName, 'x'}); err == nil {
		t.Fatal("DecodeRequestChangePetName: want error on unterminated string")
	}
}

// ---- from shop_trade_test.go ----
func TestDecodeTradeRequest(t *testing.T) {
	payload := []byte{OpcodeTradeRequest, 0x04, 0x03, 0x02, 0x01}

	got, err := DecodeTradeRequest(payload)
	if err != nil {
		t.Fatalf("DecodeTradeRequest: %v", err)
	}
	want := TradeRequest{ObjectID: 0x01020304}
	if got != want {
		t.Fatalf("DecodeTradeRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeAddTradeItem(t *testing.T) {
	payload := []byte{
		OpcodeAddTradeItem,
		0x01, 0x00, 0x00, 0x00,
		0x2c, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x00, 0x00,
	}

	got, err := DecodeAddTradeItem(payload)
	if err != nil {
		t.Fatalf("DecodeAddTradeItem: %v", err)
	}
	want := AddTradeItem{TradeID: 1, ObjectID: 300, Count: 5}
	if got != want {
		t.Fatalf("DecodeAddTradeItem = %+v, want %+v", got, want)
	}
}

func TestDecodeTradeDone(t *testing.T) {
	payload := []byte{OpcodeTradeDone, 0x01, 0x00, 0x00, 0x00}

	got, err := DecodeTradeDone(payload)
	if err != nil {
		t.Fatalf("DecodeTradeDone: %v", err)
	}
	want := TradeDone{Response: 1}
	if got != want {
		t.Fatalf("DecodeTradeDone = %+v, want %+v", got, want)
	}
}

func TestDecodeAnswerTradeRequest(t *testing.T) {
	payload := []byte{OpcodeAnswerTradeRequest, 0x00, 0x00, 0x00, 0x00}

	got, err := DecodeAnswerTradeRequest(payload)
	if err != nil {
		t.Fatalf("DecodeAnswerTradeRequest: %v", err)
	}
	want := AnswerTradeRequest{Response: 0}
	if got != want {
		t.Fatalf("DecodeAnswerTradeRequest = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestShortCutReg(t *testing.T) {
	payload := []byte{
		OpcodeRequestShortCutReg,
		0x02, 0x00, 0x00, 0x00, // skill
		0x0f, 0x00, 0x00, 0x00, // slot 3, page 1
		0xf8, 0x00, 0x00, 0x00, // skill id
		0x01, 0x00, 0x00, 0x00, // character type
	}

	got, err := DecodeRequestShortCutReg(payload)
	if err != nil {
		t.Fatalf("DecodeRequestShortCutReg: %v", err)
	}
	want := RequestShortCutReg{Type: 2, Slot: 3, Page: 1, ID: 248, CharacterType: 1}
	if got != want {
		t.Fatalf("DecodeRequestShortCutReg = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestShortCutDel(t *testing.T) {
	payload := []byte{OpcodeRequestShortCutDel, 0x0f, 0x00, 0x00, 0x00}

	got, err := DecodeRequestShortCutDel(payload)
	if err != nil {
		t.Fatalf("DecodeRequestShortCutDel: %v", err)
	}
	want := RequestShortCutDel{Slot: 3, Page: 1}
	if got != want {
		t.Fatalf("DecodeRequestShortCutDel = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestBuyItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestBuyItem,
		0x65, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x39, 0x30, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0x57, 0x04, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestBuyItem(payload, defaultMaxItemInPacket)
	if err != nil {
		t.Fatalf("DecodeRequestBuyItem: %v", err)
	}
	want := RequestBuyItem{ListID: 101, Items: []BuyItemRequest{
		{ItemID: 12345, Count: 3},
		{ItemID: 1111, Count: 1},
	}}
	if got.ListID != want.ListID || len(got.Items) != len(want.Items) || got.Items[0] != want.Items[0] || got.Items[1] != want.Items[1] {
		t.Fatalf("DecodeRequestBuyItem = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestSellItem(t *testing.T) {
	payload := []byte{
		OpcodeRequestSellItem,
		0xc8, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0xf4, 0x01, 0x00, 0x00,
		0x39, 0x30, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
		0xf5, 0x01, 0x00, 0x00,
		0x57, 0x04, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}

	got, err := DecodeRequestSellItem(payload, defaultMaxItemInPacket)
	if err != nil {
		t.Fatalf("DecodeRequestSellItem: %v", err)
	}
	want := RequestSellItem{ListID: 200, Items: []SellItemRequest{
		{ObjectID: 500, ItemID: 12345, Count: 3},
		{ObjectID: 501, ItemID: 1111, Count: 1},
	}}
	if got.ListID != want.ListID || len(got.Items) != len(want.Items) || got.Items[0] != want.Items[0] || got.Items[1] != want.Items[1] {
		t.Fatalf("DecodeRequestSellItem = %+v, want %+v", got, want)
	}
}

func TestDecodeShopTradeShort(t *testing.T) {
	if _, err := DecodeTradeRequest([]byte{OpcodeTradeRequest, 1}); err == nil {
		t.Fatal("DecodeTradeRequest: want error on short payload")
	}
	if _, err := DecodeAddTradeItem([]byte{OpcodeAddTradeItem, 1}); err == nil {
		t.Fatal("DecodeAddTradeItem: want error on short payload")
	}
	if _, err := DecodeTradeDone([]byte{OpcodeTradeDone, 1}); err == nil {
		t.Fatal("DecodeTradeDone: want error on short payload")
	}
	if _, err := DecodeAnswerTradeRequest([]byte{OpcodeAnswerTradeRequest, 1}); err == nil {
		t.Fatal("DecodeAnswerTradeRequest: want error on short payload")
	}
	if _, err := DecodeRequestShortCutReg([]byte{OpcodeRequestShortCutReg, 1}); err == nil {
		t.Fatal("DecodeRequestShortCutReg: want error on short payload")
	}
	if _, err := DecodeRequestShortCutDel([]byte{OpcodeRequestShortCutDel, 1}); err == nil {
		t.Fatal("DecodeRequestShortCutDel: want error on short payload")
	}
	if _, err := DecodeRequestBuyItem([]byte{OpcodeRequestBuyItem, 1}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on short payload")
	}
	if _, err := DecodeRequestSellItem([]byte{OpcodeRequestSellItem, 1}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on short payload")
	}
}

func TestDecodeShopTradeRejectsMalformedLists(t *testing.T) {
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item count")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on mismatched row length")
	}
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item id")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}, defaultMaxItemInPacket); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on zero count")
	}
}

// TestItemListDecodersFollowConfiguredCap pins every item-list decoder to
// the configured cap it is handed (max of the configured inventory sizes,
// not a constant): exactly maxItems well-formed rows decode, one more row is
// rejected as a plain validation error, never as a short packet.
func TestItemListDecodersFollowConfiguredCap(t *testing.T) {
	const maxItems = 117
	rows := func(n, fields int) []byte {
		var b []byte
		for range n * fields {
			b = binary.LittleEndian.AppendUint32(b, 1)
		}
		return b
	}
	withCount := func(head []byte, n, fields int) []byte {
		b := binary.LittleEndian.AppendUint32(append([]byte(nil), head...), uint32(n))
		return append(b, rows(n, fields)...)
	}
	decoders := []struct {
		name   string
		fields int
		head   []byte
		decode func([]byte, int) (int, error)
	}{
		{"RequestBuyItem", 2, []byte{OpcodeRequestBuyItem, 1, 0, 0, 0}, func(p []byte, m int) (int, error) {
			r, err := DecodeRequestBuyItem(p, m)
			return len(r.Items), err
		}},
		{"RequestSellItem", 3, []byte{OpcodeRequestSellItem, 1, 0, 0, 0}, func(p []byte, m int) (int, error) {
			r, err := DecodeRequestSellItem(p, m)
			return len(r.Items), err
		}},
		{"SendWarehouseDepositList", 2, []byte{OpcodeSendWarehouseDeposit}, func(p []byte, m int) (int, error) {
			r, err := DecodeSendWarehouseDepositList(p, m)
			return len(r.Items), err
		}},
		{"SendWarehouseWithdrawList", 2, []byte{OpcodeSendWarehouseWithdraw}, func(p []byte, m int) (int, error) {
			r, err := DecodeSendWarehouseWithdrawList(p, m)
			return len(r.Items), err
		}},
		{"RequestPackageSend", 2, []byte{OpcodeRequestPackageSend, 1, 0, 0, 0}, func(p []byte, m int) (int, error) {
			r, err := DecodeRequestPackageSend(p, m)
			return len(r.Items), err
		}},
	}
	for _, d := range decoders {
		t.Run(d.name, func(t *testing.T) {
			n, err := d.decode(withCount(d.head, maxItems, d.fields), maxItems)
			if err != nil || n != maxItems {
				t.Fatalf("%d rows at cap %d = %d rows, %v; want all accepted", maxItems, maxItems, n, err)
			}
			_, err = d.decode(withCount(d.head, maxItems+1, d.fields), maxItems)
			if err == nil {
				t.Fatalf("%d rows at cap %d accepted, want rejected", maxItems+1, maxItems)
			}
			if errors.Is(err, wire.ErrShortPacket) {
				t.Fatalf("over-cap error = %v, want a non-short-packet validation error", err)
			}
		})
	}
}

// ---- from variation_test.go ----
func TestDecodeVariationRequests(t *testing.T) {
	target, err := DecodeRequestConfirmTargetItem([]byte{
		OpcodeExtended,
		0x29, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmTargetItem: %v", err)
	}
	if target != (RequestConfirmTargetItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmTargetItem = %+v, want ObjectID 1000", target)
	}

	refiner, err := DecodeRequestConfirmRefinerItem([]byte{
		OpcodeExtended,
		0x2a, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmRefinerItem: %v", err)
	}
	if refiner != (RequestConfirmRefinerItem{TargetObjectID: 1000, RefinerObjectID: 2000}) {
		t.Fatalf("DecodeRequestConfirmRefinerItem = %+v, want target 1000 refiner 2000", refiner)
	}

	gemstone, err := DecodeRequestConfirmGemStone([]byte{
		OpcodeExtended,
		0x2b, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
		0xb8, 0x0b, 0x00, 0x00,
		0x24, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmGemStone: %v", err)
	}
	wantGemstone := RequestConfirmGemStone{
		TargetObjectID:   1000,
		RefinerObjectID:  2000,
		GemstoneObjectID: 3000,
		GemstoneCount:    36,
	}
	if gemstone != wantGemstone {
		t.Fatalf("DecodeRequestConfirmGemStone = %+v, want %+v", gemstone, wantGemstone)
	}

	cancel, err := DecodeRequestConfirmCancelItem([]byte{
		OpcodeExtended,
		0x2d, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmCancelItem: %v", err)
	}
	if cancel != (RequestConfirmCancelItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmCancelItem = %+v, want ObjectID 1000", cancel)
	}
	refine, err := DecodeRequestRefine([]byte{
		OpcodeExtended,
		0x2c, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
		0xb8, 0x0b, 0x00, 0x00,
		0x14, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestRefine: %v", err)
	}
	if want := (RequestRefine{TargetObjectID: 1000, RefinerObjectID: 2000, GemstoneObjectID: 3000, GemstoneCount: 20}); refine != want {
		t.Fatalf("DecodeRequestRefine = %+v, want %+v", refine, want)
	}

	refineCancel, err := DecodeRequestRefineCancel([]byte{
		OpcodeExtended,
		0x2e, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestRefineCancel: %v", err)
	}
	if refineCancel != (RequestRefineCancel{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestRefineCancel = %+v, want ObjectID 1000", refineCancel)
	}
}

func TestDecodeVariationRequestsShort(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x29, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x2a, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x2b, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on short payload")
	}
	if _, err := DecodeRequestRefine([]byte{OpcodeExtended, 0x2c, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestRefine: want error on short payload")
	}
	if _, err := DecodeRequestRefineCancel([]byte{OpcodeExtended, 0x2e, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestRefineCancel: want error on short payload")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x2d, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on short payload")
	}
}

func TestDecodeVariationRequestsWrongExtendedOpcode(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x2a, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on wrong extended opcode")
	}
}
