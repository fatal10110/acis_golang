package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExtended is the first byte for game server packets with a
// little-endian uint16 sub-opcode.
const OpcodeExtended = 0xfe

// Extended server packet opcodes.
const (
	OpcodeExSendManorList             uint16 = 0x001b
	OpcodeExAutoSoulShot              uint16 = 0x0012
	OpcodeExMailArrived               uint16 = 0x002d
	OpcodeExStorageMaxCount           uint16 = 0x002e
	OpcodeExPledgeCrestLarge          uint16 = 0x0028
	OpcodeExPledgeSkillList           uint16 = 0x0039
	OpcodeExCursedWeaponList          uint16 = 0x0045
	OpcodeExCursedWeaponLocation      uint16 = 0x0046
	OpcodeExUseSharedGroupItem        uint16 = 0x0049
	OpcodeExShowVariationMakeWindow   uint16 = 0x0050
	OpcodeExShowVariationCancelWindow uint16 = 0x0051
	OpcodeExConfirmVariationItem      uint16 = 0x0052
	OpcodeExConfirmVariationRefiner   uint16 = 0x0053
	OpcodeExConfirmVariationGemstone  uint16 = 0x0054
	OpcodeExVariationResult           uint16 = 0x0055
	OpcodeExConfirmCancelItem         uint16 = 0x0056
	OpcodeExVariationCancelResult     uint16 = 0x0057
)

var manorNames = [...]string{
	"gludio",
	"dion",
	"giran",
	"oren",
	"aden",
	"innadril",
	"goddard",
	"rune",
	"schuttgart",
}

// FrameExSendManorList builds the static manor list packet requested while
// the client loads into the world.
func FrameExSendManorList() wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExSendManorList)
	w.WriteInt32(int32(len(manorNames)))
	for i, name := range manorNames {
		w.WriteInt32(int32(i + 1))
		w.WriteString(name)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
