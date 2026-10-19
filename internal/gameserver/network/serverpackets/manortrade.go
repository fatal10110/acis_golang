package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// Opcodes of the manor manager's trade windows.
const (
	OpcodeBuyListSeed                    = 0xe8
	OpcodeSellListProcure                = 0xe9
	OpcodeExShowSellCropList      uint16 = 0x0021
	OpcodeExShowProcureCropDetail uint16 = 0x0022
)

// FrameBuyListSeed is the seed purchase window of castle manorID: each seed
// it sells with seeds left, at its price, for a player holding money adena.
// A castle selling none writes no row count.
func FrameBuyListSeed(money, manorID int32, seeds []ManorListing) wire.Frame {
	w := newFrameWriter(OpcodeBuyListSeed)
	w.WriteInt32(money)
	w.WriteInt32(manorID)
	if len(seeds) > 0 {
		w.WriteUint16(uint16(len(seeds)))
		for _, s := range seeds {
			w.WriteUint16(4)
			w.WriteInt32(s.ID)
			w.WriteInt32(s.ID)
			w.WriteInt32(s.Amount)
			w.WriteUint16(4)
			w.WriteUint16(0)
			w.WriteInt32(s.Price)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// ManorHeldCrop is a crop the player holds, shown in the crop sale window:
// the item, how many it holds, the crop's seed, and what the castle buys of
// it, nil when it buys none.
type ManorHeldCrop struct {
	ObjectID, CropID, Count int32
	Seed                    manor.Seed
	Procure                 *ManorListing
}

// FrameExShowSellCropList is the crop sale window at castle manorID's manor
// manager: each crop the player holds with its seed's level and reward
// items, then what the castle buys of it, or nothing (manor -1), and the
// count held.
func FrameExShowSellCropList(manorID int32, crops []ManorHeldCrop) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowSellCropList)
	w.WriteInt32(manorID)
	w.WriteInt32(int32(len(crops)))
	for _, c := range crops {
		w.WriteInt32(c.ObjectID)
		w.WriteInt32(c.CropID)
		w.WriteInt32(int32(c.Seed.Level))
		w.WriteUint8(1)
		w.WriteInt32(int32(c.Seed.Reward1))
		w.WriteUint8(1)
		w.WriteInt32(int32(c.Seed.Reward2))
		if p := c.Procure; p != nil {
			w.WriteInt32(manorID)
			w.WriteInt32(p.Amount)
			w.WriteInt32(p.Price)
			w.WriteUint8(byte(p.Reward))
		} else {
			w.WriteInt32(-1)
			w.WriteInt32(0)
			w.WriteInt32(0)
			w.WriteUint8(0)
		}
		w.WriteInt32(c.Count)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// ManorCastleProcure is what castle CastleID buys of a crop: Amount still
// wanted at Price, paid with reward type Reward.
type ManorCastleProcure struct {
	CastleID, Amount, Price, Reward int32
}

// FrameExShowProcureCropDetail is the crop sale detail window of cropID:
// each castle still buying it, with how many it wants, its price and
// reward type.
func FrameExShowProcureCropDetail(cropID int32, castles []ManorCastleProcure) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowProcureCropDetail)
	w.WriteInt32(cropID)
	w.WriteInt32(int32(len(castles)))
	for _, c := range castles {
		w.WriteInt32(c.CastleID)
		w.WriteInt32(c.Amount)
		w.WriteInt32(c.Price)
		w.WriteUint8(byte(c.Reward))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// ProcureSale is a held item a castle buys: the item and how many the
// castle still wants.
type ProcureSale struct {
	Item   *item.Instance
	Amount int32
}

// FrameSellListProcure is the procure window: each held crop a castle
// still buys, with how many it wants, for a player holding money adena. A
// crop whose template templates no longer holds (one a reload removed) is
// left out. No window the server opens sends it.
func FrameSellListProcure(money int32, rows []ProcureSale, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeSellListProcure)
	w.WriteInt32(money)
	w.WriteInt32(0)
	countOffset, err := reserveUint16Count(w, len(rows))
	if err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	var written int
	for _, r := range rows {
		st := r.Item.Snapshot()
		tmpl, ok := templates.Get(st.TemplateID)
		if !ok {
			continue
		}
		written++
		category, subCategory := tmpl.Category()
		w.WriteUint16(uint16(category))
		w.WriteInt32(st.ObjectID)
		w.WriteInt32(st.TemplateID)
		w.WriteInt32(r.Amount)
		w.WriteUint16(uint16(subCategory))
		w.WriteUint16(0)
		w.WriteInt32(0)
	}
	w.PatchUint16(countOffset, uint16(written))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}
