package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodePledgeCrest is the wire opcode for PledgeCrest.
	OpcodePledgeCrest = 0x6c
	// OpcodeAllyCrest is the wire opcode for AllyCrest.
	OpcodeAllyCrest = 0xae
)

// FramePledgeCrest builds a pledge crest data response. Missing crest data is
// represented by a zero byte count.
func FramePledgeCrest(crestID int32, data []byte) wire.Frame {
	w := newFrameWriter(OpcodePledgeCrest)
	w.WriteInt32(crestID)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameAllyCrest builds an alliance crest data response.
func FrameAllyCrest(crestID int32, data []byte) wire.Frame {
	w := newFrameWriter(OpcodeAllyCrest)
	w.WriteInt32(crestID)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
