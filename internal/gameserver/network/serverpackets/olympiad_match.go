package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Olympiad match server packet sub-opcodes.
const (
	OpcodeExOlympiadUserInfo uint16 = 0x0029
	OpcodeExOlympiadMode     uint16 = 0x002b
	OpcodeExOlympiadMatchEnd uint16 = 0x002c
)

// Olympiad modes ExOlympiadMode switches the receiver's client to.
const (
	OlympiadModeNone  = 0
	OlympiadModeSide1 = 1
	OlympiadModeSide2 = 2
)

// Olympiad match system message ids.
const (
	SystemMessageEnterOlympiadStadiumInS1Seconds = 1492 // number: seconds
	SystemMessageOlympiadOpponentEndedTheGame    = 1493 // no parameter
	SystemMessageOlympiadOpponentUnfit           = 1494 // no parameter
	SystemMessageOlympiadGameStartsInS1Seconds   = 1495 // number: seconds
	SystemMessageOlympiadStartsTheGame           = 1496 // no parameter
	SystemMessageS1HasWonTheGame                 = 1497 // text: the winner
	SystemMessageOlympiadGameEndedInATie         = 1498 // no parameter
	SystemMessageMovedToTownInS1Seconds          = 1499 // number: seconds
	SystemMessageS1HasGainedS2OlympiadPoints     = 1657 // text then number
	SystemMessageS1HasLostS2OlympiadPoints       = 1658 // text then number
	SystemMessageSubJobCannotJoinOlympiad        = 1692 // no parameter
	SystemMessageCannotJoinOlympiadWhileDead     = 1858 // no parameter
	SystemMessageEnteredCombatZone               = 283  // no parameter
	SystemMessageLeftCombatZone                  = 284  // no parameter
)

// FrameExOlympiadMode switches the receiver's client into an Olympiad mode:
// a competitor's side, or none.
func FrameExOlympiadMode(mode int) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExOlympiadMode)
	w.WriteUint8(uint8(mode))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExOlympiadMatchEnd closes the receiver's Olympiad match window.
func FrameExOlympiadMatchEnd() wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExOlympiadMatchEnd)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// OlympiadUserInfo is a competitor as an Olympiad match window shows it.
type OlympiadUserInfo struct {
	Side      int
	ObjectID  int32
	Name      string
	ClassID   int32
	HP, MaxHP int32
	CP, MaxCP int32
}

// FrameExOlympiadUserInfo shows one competitor's gauges in the receiver's
// Olympiad match window.
func FrameExOlympiadUserInfo(u OlympiadUserInfo) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExOlympiadUserInfo)
	w.WriteUint8(uint8(u.Side))
	w.WriteInt32(u.ObjectID)
	w.WriteString(u.Name)
	w.WriteInt32(u.ClassID)
	w.WriteInt32(u.HP)
	w.WriteInt32(u.MaxHP)
	w.WriteInt32(u.CP)
	w.WriteInt32(u.MaxCP)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
