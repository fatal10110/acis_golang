package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Sky packets switch every receiving client's sky rendering between day and
// night. Both carry only their opcode. The server sends them only on an
// explicit operator request; the game clock's own day/night crossings do not
// send them.
const (
	// OpcodeSunRise is the wire opcode for SunRise, which turns the
	// client's sky to day.
	OpcodeSunRise = 0x1c
	// OpcodeSunSet is the wire opcode for SunSet, which turns the client's
	// sky to night.
	OpcodeSunSet = 0x1d
)

// FrameSunRise builds the SunRise packet as an owned frame.
func FrameSunRise() wire.Frame {
	w := newFrameWriter(OpcodeSunRise)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSunSet builds the SunSet packet as an owned frame.
func FrameSunSet() wire.Frame {
	w := newFrameWriter(OpcodeSunSet)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
