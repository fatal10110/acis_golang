package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// Extended sub-opcodes of the manor information and setting windows.
const (
	OpcodeExShowSeedInfo         uint16 = 0x001c
	OpcodeExShowCropInfo         uint16 = 0x001d
	OpcodeExShowManorDefaultInfo uint16 = 0x001e
	OpcodeExShowSeedSetting      uint16 = 0x001f
	OpcodeExShowCropSetting      uint16 = 0x0020
)

// ManorListing is one seed a castle sells, or one crop it buys, over a
// manor period: StartAmount at Price, Amount of them left, a crop paid
// with reward item Reward of its seed. Seed is the seed it belongs to, the
// zero Seed when the manor table holds none.
type ManorListing struct {
	ID, Amount, StartAmount, Price, Reward int32
	Seed                                   manor.Seed
}

// ManorPeriodSetting is what a castle set for one seed or crop over a
// period: StartAmount at Price, a crop paid with reward type Reward. The
// zero value is nothing set.
type ManorPeriodSetting struct {
	StartAmount, Price, Reward int32
}

// ManorSetting is one row of a castle's seed or crop setting window: the
// seed, the most the castle may sell or buy of it over a period, and what
// it set for the running and the next period.
type ManorSetting struct {
	Seed          manor.Seed
	Limit         int32
	Current, Next ManorPeriodSetting
}

// FrameExShowSeedInfo is the seeds castle manorID sells over a period,
// each with its seed's level and reward items; hideButtons hides the
// window's purchase buttons.
func FrameExShowSeedInfo(manorID int32, hideButtons bool, seeds []ManorListing) wire.Frame {
	w := manorInfoHeader(OpcodeExShowSeedInfo, manorID, hideButtons, len(seeds))
	for _, s := range seeds {
		w.WriteInt32(s.ID)
		w.WriteInt32(s.Amount)
		w.WriteInt32(s.StartAmount)
		w.WriteInt32(s.Price)
		writeManorSeedRewards(w, s.Seed)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExShowCropInfo is the crops castle manorID buys over a period, each
// with its reward type and its seed's level and reward items; hideButtons
// hides the window's sale buttons.
func FrameExShowCropInfo(manorID int32, hideButtons bool, crops []ManorListing) wire.Frame {
	w := manorInfoHeader(OpcodeExShowCropInfo, manorID, hideButtons, len(crops))
	for _, c := range crops {
		w.WriteInt32(c.ID)
		w.WriteInt32(c.Amount)
		w.WriteInt32(c.StartAmount)
		w.WriteInt32(c.Price)
		w.WriteUint8(byte(c.Reward))
		writeManorSeedRewards(w, c.Seed)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// manorInfoHeader starts a seed or crop information window of n rows.
func manorInfoHeader(opcode uint16, manorID int32, hideButtons bool, n int) *wire.Writer {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(opcode)
	w.WriteUint8(boolUint8(hideButtons))
	w.WriteInt32(manorID)
	w.WriteInt32(0)
	w.WriteInt32(int32(n))
	return w
}

// writeManorSeedRewards writes s's level and its two reward items.
func writeManorSeedRewards(w *wire.Writer, s manor.Seed) {
	w.WriteInt32(int32(s.Level))
	w.WriteUint8(1)
	w.WriteInt32(int32(s.Reward1))
	w.WriteUint8(1)
	w.WriteInt32(int32(s.Reward2))
}

// FrameExShowManorDefaultInfo is the manor's crops, one seed each: the
// crop, its level, the seed's and the crop's reference prices and the
// reward items. hideButtons hides the window's buttons.
func FrameExShowManorDefaultInfo(hideButtons bool, crops []manor.Seed) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowManorDefaultInfo)
	w.WriteUint8(boolUint8(hideButtons))
	w.WriteInt32(int32(len(crops)))
	for _, s := range crops {
		w.WriteInt32(int32(s.CropID))
		w.WriteInt32(int32(s.Level))
		w.WriteInt32(s.SeedReferencePrice)
		w.WriteInt32(s.CropReferencePrice)
		w.WriteUint8(1)
		w.WriteInt32(int32(s.Reward1))
		w.WriteUint8(1)
		w.WriteInt32(int32(s.Reward2))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExShowSeedSetting is castle manorID's seed setting window: per
// seed of its manor the seed, its level and reward items, how many the
// castle may sell, the seed's reference price and price bounds, and the
// amount and price set for the running and the next period.
func FrameExShowSeedSetting(manorID int32, rows []ManorSetting) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowSeedSetting)
	w.WriteInt32(manorID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(int32(r.Seed.SeedID))
		writeManorSeedRewards(w, r.Seed)
		w.WriteInt32(r.Limit)
		w.WriteInt32(r.Seed.SeedReferencePrice)
		w.WriteInt32(r.Seed.SeedMinPrice())
		w.WriteInt32(r.Seed.SeedMaxPrice())
		for _, p := range []ManorPeriodSetting{r.Current, r.Next} {
			w.WriteInt32(p.StartAmount)
			w.WriteInt32(p.Price)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExShowCropSetting is castle manorID's crop setting window: per seed
// of its manor the crop, its level and reward items, how many the castle
// may buy, the crop's price bounds, and the amount, price and reward type
// set for the running and the next period. The window shows no reference
// price: that field is 0.
func FrameExShowCropSetting(manorID int32, rows []ManorSetting) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExShowCropSetting)
	w.WriteInt32(manorID)
	w.WriteInt32(int32(len(rows)))
	for _, r := range rows {
		w.WriteInt32(int32(r.Seed.CropID))
		writeManorSeedRewards(w, r.Seed)
		w.WriteInt32(r.Limit)
		w.WriteInt32(0)
		w.WriteInt32(r.Seed.CropMinPrice())
		w.WriteInt32(r.Seed.CropMaxPrice())
		for _, p := range []ManorPeriodSetting{r.Current, r.Next} {
			w.WriteInt32(p.StartAmount)
			w.WriteInt32(p.Price)
			w.WriteUint8(byte(p.Reward))
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
