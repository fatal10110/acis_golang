package clientpackets

import "testing"

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

	got, err := DecodeRequestBuyItem(payload)
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

	got, err := DecodeRequestSellItem(payload)
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
	if _, err := DecodeRequestBuyItem([]byte{OpcodeRequestBuyItem, 1}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on short payload")
	}
	if _, err := DecodeRequestSellItem([]byte{OpcodeRequestSellItem, 1}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on short payload")
	}
}

func TestDecodeShopTradeRejectsMalformedLists(t *testing.T) {
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item count")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on mismatched row length")
	}
	if _, err := DecodeRequestBuyItem([]byte{
		OpcodeRequestBuyItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestBuyItem: want error on zero item id")
	}
	if _, err := DecodeRequestSellItem([]byte{
		OpcodeRequestSellItem,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}); err == nil {
		t.Fatal("DecodeRequestSellItem: want error on zero count")
	}
}
