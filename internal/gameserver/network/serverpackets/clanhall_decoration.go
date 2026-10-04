package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeClanHallDecoration is the wire opcode for ClanHallDecoration.
const OpcodeClanHallDecoration = 0xf7

// ClanHallDecoration is the decoration level each rented function shows
// inside a clan hall; 0 shows none.
type ClanHallDecoration struct {
	HallID       int32
	RestoreHP    int
	RestoreMP    int
	RestoreExp   int
	Teleport     int
	Curtains     int
	SupportMagic int
	Fixtures     int
	CreateItem   int
}

// FrameClanHallDecoration builds the clan hall decoration packet: the hall
// id, then one level byte per decoration slot in the client's order, the
// slots no function fills (statue, crystal, hangings, flag) left at 0.
func FrameClanHallDecoration(d ClanHallDecoration) wire.Frame {
	w := newFrameWriter(OpcodeClanHallDecoration)
	w.WriteInt32(d.HallID)
	for _, level := range [...]int{
		d.RestoreHP, d.RestoreMP, 0, d.RestoreExp, d.Teleport, 0,
		d.Curtains, 0, d.SupportMagic, 0, d.Fixtures, d.CreateItem,
	} {
		w.WriteUint8(byte(level))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
