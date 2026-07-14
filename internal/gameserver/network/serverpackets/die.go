package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeDie is the wire opcode for Die.
const OpcodeDie = 0x06

// DieOptions controls which death-window restart choices the client enables.
type DieOptions struct {
	ClanHall bool
	Castle   bool
	SiegeHQ  bool
	Sweep    bool
	FixedRes bool
}

// FrameDie builds the death packet for a creature that is already dead.
func FrameDie(objectID int32, opts DieOptions) wire.Frame {
	w := newFrameWriter(OpcodeDie)
	w.WriteInt32(objectID)
	w.WriteInt32(1)
	w.WriteInt32(boolInt32(opts.ClanHall))
	w.WriteInt32(boolInt32(opts.Castle))
	w.WriteInt32(boolInt32(opts.SiegeHQ))
	w.WriteInt32(boolInt32(opts.Sweep))
	w.WriteInt32(boolInt32(opts.FixedRes))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
