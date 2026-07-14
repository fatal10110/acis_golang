package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// OpcodeHennaInfo is the wire opcode for HennaInfo.
const OpcodeHennaInfo = 0xe4

// FrameHennaInfo builds the empty henna list currently available to a
// character.
func FrameHennaInfo(classID int) wire.Frame {
	w := newFrameWriter(OpcodeHennaInfo)
	for i := 0; i < 6; i++ {
		w.WriteUint8(0)
	}
	w.WriteInt32(int32(maxHennaSlots(classID)))
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func maxHennaSlots(classID int) int {
	level := 0
	for id := classID; ; {
		parent, ok := player.ClassParent(id)
		if !ok || parent < 0 {
			break
		}
		level++
		id = parent
	}
	if level < 1 {
		return 0
	}
	if level == 1 {
		return 2
	}
	return 3
}
