package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodePledgeCrest is the wire opcode for PledgeCrest.
const OpcodePledgeCrest = 0x6c

// FramePledgeCrest builds a pledge crest data response. Missing crest data is
// represented by a zero byte count.
func FramePledgeCrest(crestID int32, data []byte) wire.Frame {
	w := newFrameWriter(OpcodePledgeCrest)
	w.WriteInt32(crestID)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
