package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeAutoAttackStop is the wire opcode that stops an actor's attack
// animation on the receiving client.
const OpcodeAutoAttackStop = 0x2c

// FrameAutoAttackStop builds the AutoAttackStop packet as an owned frame.
func FrameAutoAttackStop(objectID int32) wire.Frame {
	w := newFrameWriter(OpcodeAutoAttackStop)
	w.WriteInt32(objectID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
