package clientpackets

import "fmt"

// Extended client packet opcodes of the castle manor setting windows.
const (
	OpcodeRequestSetSeed uint16 = 0x000a
	OpcodeRequestSetCrop uint16 = 0x000b
)

const (
	// manorSetHeaderSize is the sub-opcode, manor id and row count.
	manorSetHeaderSize = 2 + 2*4
	setSeedRowSize     = 3 * 4
	setCropRowSize     = 3*4 + 1
)

// ManorSeedSetting is one seed a castle offers for the next manor period:
// Sales seeds at Price each.
type ManorSeedSetting struct {
	SeedID, Sales, Price int32
}

// RequestSetSeed sets the seeds castle ManorID sells over the next manor
// period.
type RequestSetSeed struct {
	ManorID int32
	Items   []ManorSeedSetting
}

// ManorCropSetting is one crop a castle wants for the next manor period:
// Sales crops at Price each, paid with reward type Reward.
type ManorCropSetting struct {
	CropID, Sales, Price int32
	Reward               byte
}

// RequestSetCrop sets the crops castle ManorID buys over the next manor
// period.
type RequestSetCrop struct {
	ManorID int32
	Items   []ManorCropSetting
}

// DecodeRequestSetSeed parses a raw RequestSetSeed payload (opcode byte
// included) of at most maxItems rows (player.InventorySlots.MaxItemInPacket).
// A row count out of range or not matching the bytes left, or a row naming
// no item or a negative amount or price, refuses the whole request.
func DecodeRequestSetSeed(payload []byte, maxItems int) (RequestSetSeed, error) {
	const name = "RequestSetSeed"
	r, err := newExtendedReader(payload, name, OpcodeRequestSetSeed, manorSetHeaderSize)
	if err != nil {
		return RequestSetSeed{}, err
	}
	req := RequestSetSeed{ManorID: r.ReadInt32()}
	count := r.ReadInt32()
	if err := validateShopRows(name, count, maxItems, setSeedRowSize, r.Remaining()); err != nil {
		return RequestSetSeed{}, err
	}
	req.Items = make([]ManorSeedSetting, count)
	for i := range req.Items {
		row := ManorSeedSetting{SeedID: r.ReadInt32(), Sales: r.ReadInt32(), Price: r.ReadInt32()}
		if row.SeedID < 1 || row.Sales < 0 || row.Price < 0 {
			return RequestSetSeed{}, fmt.Errorf("clientpackets: %s: invalid row seed=%d sales=%d price=%d", name, row.SeedID, row.Sales, row.Price)
		}
		req.Items[i] = row
	}
	if err := r.Err(); err != nil {
		return RequestSetSeed{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return req, nil
}

// DecodeRequestSetCrop parses a raw RequestSetCrop payload (opcode byte
// included) of at most maxItems rows, refused as DecodeRequestSetSeed
// refuses. A crop with no sales is left out of Items.
func DecodeRequestSetCrop(payload []byte, maxItems int) (RequestSetCrop, error) {
	const name = "RequestSetCrop"
	r, err := newExtendedReader(payload, name, OpcodeRequestSetCrop, manorSetHeaderSize)
	if err != nil {
		return RequestSetCrop{}, err
	}
	req := RequestSetCrop{ManorID: r.ReadInt32()}
	count := r.ReadInt32()
	if err := validateShopRows(name, count, maxItems, setCropRowSize, r.Remaining()); err != nil {
		return RequestSetCrop{}, err
	}
	req.Items = make([]ManorCropSetting, 0, count)
	for range count {
		row := ManorCropSetting{CropID: r.ReadInt32(), Sales: r.ReadInt32(), Price: r.ReadInt32(), Reward: r.ReadUint8()}
		if row.CropID < 1 || row.Sales < 0 || row.Price < 0 {
			return RequestSetCrop{}, fmt.Errorf("clientpackets: %s: invalid row crop=%d sales=%d price=%d", name, row.CropID, row.Sales, row.Price)
		}
		if row.Sales > 0 {
			req.Items = append(req.Items, row)
		}
	}
	if err := r.Err(); err != nil {
		return RequestSetCrop{}, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return req, nil
}
