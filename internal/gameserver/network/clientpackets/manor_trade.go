package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// Client packet opcodes of the manor manager's trade windows.
const (
	OpcodeRequestBuyProcure             = 0xc3
	OpcodeRequestBuySeed                = 0xc4
	OpcodeRequestProcureCropList uint16 = 0x0009
)

const (
	// manorTradeHeaderSize is the manor id and row count.
	manorTradeHeaderSize = 2 * 4
	// buySeedRowSize is a seed purchase row: seed id and count.
	buySeedRowSize = 2 * 4
	// procureCropRowSize is a crop sale row: object id, crop id, manor id
	// and count.
	procureCropRowSize = 4 * 4
	// buyProcureRowCheck is the row size a RequestBuyProcure count is
	// checked against; each row then reads buyProcureRowSize bytes.
	buyProcureRowCheck = 2 * 4
	buyProcureRowSize  = 3 * 4
)

// SeedOrder is one row of a seed purchase: Count seeds of SeedID.
type SeedOrder struct {
	SeedID, Count int32
}

// RequestBuySeed buys seeds from castle ManorID's manor manager. Items is
// nil when the rows were refused: the request still runs, to be answered
// ActionFailed.
type RequestBuySeed struct {
	ManorID int32
	Items   []SeedOrder
}

// DecodeRequestBuySeed parses a raw RequestBuySeed payload (opcode byte
// included) of at most maxItems rows (player.InventorySlots.MaxItemInPacket).
// A row count out of range or not matching the bytes left, or a row naming
// no seed or a count under 1, leaves Items nil; only a payload too short
// for its header is an error.
func DecodeRequestBuySeed(payload []byte, maxItems int) (RequestBuySeed, error) {
	r := newReader(payload)
	if r.Remaining() < manorTradeHeaderSize {
		return RequestBuySeed{}, fmt.Errorf("clientpackets: RequestBuySeed: need %d bytes, got %d: %w", manorTradeHeaderSize, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestBuySeed{ManorID: r.ReadInt32()}
	count := r.ReadInt32()
	if validateShopRows("RequestBuySeed", count, maxItems, buySeedRowSize, r.Remaining()) != nil {
		return req, nil
	}
	items := make([]SeedOrder, count)
	for i := range items {
		row := SeedOrder{SeedID: r.ReadInt32(), Count: r.ReadInt32()}
		if row.SeedID < 1 || row.Count < 1 {
			return req, nil
		}
		items[i] = row
	}
	req.Items = items
	return req, nil
}

// CropSale is one row of a crop sale: Count of the crop CropID held as
// item ObjectID, sold to castle ManorID.
type CropSale struct {
	ObjectID, CropID, ManorID, Count int32
}

// RequestProcureCropList sells crops to the manor manager talked to. Items
// is nil when the rows were refused: the request then does nothing.
type RequestProcureCropList struct {
	Items []CropSale
}

// DecodeRequestProcureCropList parses a raw RequestProcureCropList payload
// (extended opcode included) of at most maxItems rows. A row count out of
// range or not matching the bytes left, or a row with an object or crop id
// under 1 or a negative manor id or count, leaves Items nil; only a payload
// too short for its row count is an error.
func DecodeRequestProcureCropList(payload []byte, maxItems int) (RequestProcureCropList, error) {
	const name = "RequestProcureCropList"
	r, err := newExtendedReader(payload, name, OpcodeRequestProcureCropList, 2+4)
	if err != nil {
		return RequestProcureCropList{}, err
	}
	count := r.ReadInt32()
	if validateShopRows(name, count, maxItems, procureCropRowSize, r.Remaining()) != nil {
		return RequestProcureCropList{}, nil
	}
	items := make([]CropSale, count)
	for i := range items {
		row := CropSale{ObjectID: r.ReadInt32(), CropID: r.ReadInt32(), ManorID: r.ReadInt32(), Count: r.ReadInt32()}
		if row.ObjectID < 1 || row.CropID < 1 || row.ManorID < 0 || row.Count < 0 {
			return RequestProcureCropList{}, nil
		}
		items[i] = row
	}
	return RequestProcureCropList{Items: items}, nil
}

// ProcureOrder is one row of a RequestBuyProcure: Count of item ItemID.
type ProcureOrder struct {
	ItemID, Count int32
}

// RequestBuyProcure is the procure window's sale, which no window the
// server opens sends.
type RequestBuyProcure struct {
	ManorID int32
	Items   []ProcureOrder
}

// DecodeRequestBuyProcure parses a raw RequestBuyProcure payload of at most
// maxItems rows. The row count must match 8-byte rows while each row reads
// 12 bytes (an unused service word, the item id and the count), so no
// request decodes: a refused count, or a row naming no item or a count
// under 1, is an error, and rows running out of bytes are ErrShortPacket.
func DecodeRequestBuyProcure(payload []byte, maxItems int) (RequestBuyProcure, error) {
	const name = "RequestBuyProcure"
	r := newReader(payload)
	if r.Remaining() < manorTradeHeaderSize {
		return RequestBuyProcure{}, fmt.Errorf("clientpackets: %s: need %d bytes, got %d: %w", name, manorTradeHeaderSize, r.Remaining(), wire.ErrShortPacket)
	}
	req := RequestBuyProcure{ManorID: r.ReadInt32()}
	count := r.ReadInt32()
	if err := validateShopRows(name, count, maxItems, buyProcureRowCheck, r.Remaining()); err != nil {
		return RequestBuyProcure{}, err
	}
	req.Items = make([]ProcureOrder, 0, count)
	for range count {
		if r.Remaining() < buyProcureRowSize {
			return RequestBuyProcure{}, fmt.Errorf("clientpackets: %s: row needs %d bytes, got %d: %w", name, buyProcureRowSize, r.Remaining(), wire.ErrShortPacket)
		}
		r.ReadInt32()
		row := ProcureOrder{ItemID: r.ReadInt32(), Count: r.ReadInt32()}
		if row.ItemID < 1 || row.Count < 1 {
			return RequestBuyProcure{}, fmt.Errorf("clientpackets: %s: invalid row item=%d count=%d", name, row.ItemID, row.Count)
		}
		req.Items = append(req.Items, row)
	}
	return req, nil
}
