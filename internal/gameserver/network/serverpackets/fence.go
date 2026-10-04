package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExColosseumFenceInfo is the extended sub-opcode for
// ExColosseumFenceInfo, which shows a fence.
const OpcodeExColosseumFenceInfo uint16 = 0x0009

// FenceInfo is what ExColosseumFenceInfo shows of a fence: the object id it
// is shown under, its type (1 corner columns only, 2 columns joined by
// fences), position and size.
type FenceInfo struct {
	ObjectID     int32
	Type         int32
	X, Y, Z      int32
	SizeX, SizeY int32
}

// FrameExColosseumFenceInfo builds the ExColosseumFenceInfo packet as an
// owned frame.
func FrameExColosseumFenceInfo(info FenceInfo) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExColosseumFenceInfo)
	w.WriteInt32(info.ObjectID)
	w.WriteInt32(info.Type)
	w.WriteInt32(info.X)
	w.WriteInt32(info.Y)
	w.WriteInt32(info.Z)
	w.WriteInt32(info.SizeX)
	w.WriteInt32(info.SizeY)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
