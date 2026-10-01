package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Wire opcodes of the client windows and animations an item opens or plays
// without any other effect.
const (
	OpcodeShowMiniMap    = 0x9d
	OpcodeDice           = 0xd4
	OpcodeShowCalculator = 0xdc
	OpcodeShowXMasSeal   = 0xf2
)

// Dice throw system message ids.
const (
	SystemMessageS1RolledS2                = 834 // text (roller name) then number parameter
	SystemMessageCannotThrowDiceAtThisTime = 835 // no parameter
)

// RegularMapID is the map the client's own mini-map request opens: the
// World Map item's.
const RegularMapID = 1665

// FrameShowMiniMap opens map mapID, drawn for Seven Signs period period
// (the period's ordinal: recruiting 0 through seal validation 3).
func FrameShowMiniMap(mapID, period int32) wire.Frame {
	w := newFrameWriter(OpcodeShowMiniMap)
	w.WriteInt32(mapID)
	w.WriteInt32(period)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameShowCalculator opens calculator calculatorID.
func FrameShowCalculator(calculatorID int32) wire.Frame {
	w := newFrameWriter(OpcodeShowCalculator)
	w.WriteInt32(calculatorID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameShowXMasSeal opens the seal window of item itemID.
func FrameShowXMasSeal(itemID int32) wire.Frame {
	w := newFrameWriter(OpcodeShowXMasSeal)
	w.WriteInt32(itemID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameDice plays roller objectID's throw of die itemID landing on number at
// at, the spot in front of the roller.
func FrameDice(objectID, itemID, number int32, at location.Location) wire.Frame {
	w := newFrameWriter(OpcodeDice)
	w.WriteInt32(objectID)
	w.WriteInt32(itemID)
	w.WriteInt32(number)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
