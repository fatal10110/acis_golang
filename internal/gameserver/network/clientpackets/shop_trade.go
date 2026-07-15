package clientpackets

import "fmt"

const (
	tradeRequestSize       = 4
	addTradeItemSize       = 3 * 4
	tradeDoneSize          = 4
	answerTradeRequestSize = 4
	shortcutRegSize        = 4 * 4
	shortcutDelSize        = 4
	shopBuyRowSize         = 2 * 4
	shopSellRowSize        = 3 * 4
	shopHeaderSize         = 2 * 4
)

// TradeRequest asks another player to open a direct trade.
type TradeRequest struct {
	ObjectID int32
}

// DecodeTradeRequest parses a raw TradeRequest payload (opcode byte
// included).
func DecodeTradeRequest(payload []byte) (TradeRequest, error) {
	r := newReader(payload)
	if r.Remaining() < tradeRequestSize {
		return TradeRequest{}, fmt.Errorf("clientpackets: TradeRequest: need %d bytes, got %d", tradeRequestSize, r.Remaining())
	}
	req := TradeRequest{ObjectID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return TradeRequest{}, fmt.Errorf("clientpackets: TradeRequest: %w", err)
	}
	return req, nil
}

// AddTradeItem adds one inventory item stack to the active direct-trade
// offer.
type AddTradeItem struct {
	TradeID  int32
	ObjectID int32
	Count    int32
}

// DecodeAddTradeItem parses a raw AddTradeItem payload (opcode byte
// included).
func DecodeAddTradeItem(payload []byte) (AddTradeItem, error) {
	r := newReader(payload)
	if r.Remaining() < addTradeItemSize {
		return AddTradeItem{}, fmt.Errorf("clientpackets: AddTradeItem: need %d bytes, got %d", addTradeItemSize, r.Remaining())
	}
	req := AddTradeItem{
		TradeID:  r.ReadInt32(),
		ObjectID: r.ReadInt32(),
		Count:    r.ReadInt32(),
	}
	if err := r.Err(); err != nil {
		return AddTradeItem{}, fmt.Errorf("clientpackets: AddTradeItem: %w", err)
	}
	return req, nil
}

// TradeDone confirms or cancels the active direct trade.
type TradeDone struct {
	Response int32
}

// DecodeTradeDone parses a raw TradeDone payload (opcode byte included).
func DecodeTradeDone(payload []byte) (TradeDone, error) {
	r := newReader(payload)
	if r.Remaining() < tradeDoneSize {
		return TradeDone{}, fmt.Errorf("clientpackets: TradeDone: need %d bytes, got %d", tradeDoneSize, r.Remaining())
	}
	req := TradeDone{Response: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return TradeDone{}, fmt.Errorf("clientpackets: TradeDone: %w", err)
	}
	return req, nil
}

// AnswerTradeRequest accepts or rejects a pending direct-trade request.
type AnswerTradeRequest struct {
	Response int32
}

// DecodeAnswerTradeRequest parses a raw AnswerTradeRequest payload (opcode
// byte included).
func DecodeAnswerTradeRequest(payload []byte) (AnswerTradeRequest, error) {
	r := newReader(payload)
	if r.Remaining() < answerTradeRequestSize {
		return AnswerTradeRequest{}, fmt.Errorf("clientpackets: AnswerTradeRequest: need %d bytes, got %d", answerTradeRequestSize, r.Remaining())
	}
	req := AnswerTradeRequest{Response: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return AnswerTradeRequest{}, fmt.Errorf("clientpackets: AnswerTradeRequest: %w", err)
	}
	return req, nil
}

// RequestShortCutReg adds or replaces one client shortcut bar entry.
type RequestShortCutReg struct {
	Type          int32
	Slot          int32
	Page          int32
	ID            int32
	CharacterType int32
}

// DecodeRequestShortCutReg parses a raw RequestShortCutReg payload (opcode
// byte included).
func DecodeRequestShortCutReg(payload []byte) (RequestShortCutReg, error) {
	r := newReader(payload)
	if r.Remaining() < shortcutRegSize {
		return RequestShortCutReg{}, fmt.Errorf("clientpackets: RequestShortCutReg: need %d bytes, got %d", shortcutRegSize, r.Remaining())
	}
	req := RequestShortCutReg{
		Type: r.ReadInt32(),
	}
	slot := r.ReadInt32()
	req.ID = r.ReadInt32()
	req.CharacterType = r.ReadInt32()
	req.Slot = slot % 12
	req.Page = slot / 12
	if err := r.Err(); err != nil {
		return RequestShortCutReg{}, fmt.Errorf("clientpackets: RequestShortCutReg: %w", err)
	}
	return req, nil
}

// RequestShortCutDel removes one client shortcut bar entry.
type RequestShortCutDel struct {
	Slot int32
	Page int32
}

// DecodeRequestShortCutDel parses a raw RequestShortCutDel payload (opcode
// byte included).
func DecodeRequestShortCutDel(payload []byte) (RequestShortCutDel, error) {
	r := newReader(payload)
	if r.Remaining() < shortcutDelSize {
		return RequestShortCutDel{}, fmt.Errorf("clientpackets: RequestShortCutDel: need %d bytes, got %d", shortcutDelSize, r.Remaining())
	}
	slot := r.ReadInt32()
	req := RequestShortCutDel{Slot: slot % 12, Page: slot / 12}
	if err := r.Err(); err != nil {
		return RequestShortCutDel{}, fmt.Errorf("clientpackets: RequestShortCutDel: %w", err)
	}
	return req, nil
}

// BuyItemRequest is one requested item/count row in RequestBuyItem.
type BuyItemRequest struct {
	ItemID int32
	Count  int32
}

// RequestBuyItem asks the current merchant context to sell the requested
// item rows from a buylist.
type RequestBuyItem struct {
	ListID int32
	Items  []BuyItemRequest
}

// DecodeRequestBuyItem parses a raw RequestBuyItem payload (opcode byte
// included).
func DecodeRequestBuyItem(payload []byte) (RequestBuyItem, error) {
	r := newReader(payload)
	if r.Remaining() < shopHeaderSize {
		return RequestBuyItem{}, fmt.Errorf("clientpackets: RequestBuyItem: need at least %d bytes, got %d", shopHeaderSize, r.Remaining())
	}
	req := RequestBuyItem{ListID: r.ReadInt32()}
	count := r.ReadInt32()
	if err := validateShopRows("RequestBuyItem", count, shopBuyRowSize, r.Remaining()); err != nil {
		return RequestBuyItem{}, err
	}
	req.Items = make([]BuyItemRequest, count)
	for i := range req.Items {
		row := BuyItemRequest{ItemID: r.ReadInt32(), Count: r.ReadInt32()}
		if row.ItemID < 1 || row.Count < 1 {
			return RequestBuyItem{}, fmt.Errorf("clientpackets: RequestBuyItem: invalid row item=%d count=%d", row.ItemID, row.Count)
		}
		req.Items[i] = row
	}
	if err := r.Err(); err != nil {
		return RequestBuyItem{}, fmt.Errorf("clientpackets: RequestBuyItem: %w", err)
	}
	return req, nil
}

// SellItemRequest is one requested object/template/count row in
// RequestSellItem.
type SellItemRequest struct {
	ObjectID int32
	ItemID   int32
	Count    int32
}

// RequestSellItem asks the current merchant context to buy the requested
// inventory rows from the player.
type RequestSellItem struct {
	ListID int32
	Items  []SellItemRequest
}

// DecodeRequestSellItem parses a raw RequestSellItem payload (opcode byte
// included).
func DecodeRequestSellItem(payload []byte) (RequestSellItem, error) {
	r := newReader(payload)
	if r.Remaining() < shopHeaderSize {
		return RequestSellItem{}, fmt.Errorf("clientpackets: RequestSellItem: need at least %d bytes, got %d", shopHeaderSize, r.Remaining())
	}
	req := RequestSellItem{ListID: r.ReadInt32()}
	count := r.ReadInt32()
	if err := validateShopRows("RequestSellItem", count, shopSellRowSize, r.Remaining()); err != nil {
		return RequestSellItem{}, err
	}
	req.Items = make([]SellItemRequest, count)
	for i := range req.Items {
		row := SellItemRequest{
			ObjectID: r.ReadInt32(),
			ItemID:   r.ReadInt32(),
			Count:    r.ReadInt32(),
		}
		if row.ObjectID < 1 || row.ItemID < 1 || row.Count < 1 {
			return RequestSellItem{}, fmt.Errorf("clientpackets: RequestSellItem: invalid row object=%d item=%d count=%d", row.ObjectID, row.ItemID, row.Count)
		}
		req.Items[i] = row
	}
	if err := r.Err(); err != nil {
		return RequestSellItem{}, fmt.Errorf("clientpackets: RequestSellItem: %w", err)
	}
	return req, nil
}

func validateShopRows(name string, count int32, rowSize, remaining int) error {
	if count <= 0 {
		return fmt.Errorf("clientpackets: %s: invalid item count %d", name, count)
	}
	if int64(count)*int64(rowSize) != int64(remaining) {
		return fmt.Errorf("clientpackets: %s: %d item rows need %d bytes, got %d", name, count, int64(count)*int64(rowSize), remaining)
	}
	return nil
}
