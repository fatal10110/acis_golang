package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Sky packets switch every receiving client's sky rendering between day and
// night. SunRise and SunSet carry only their opcode. The server sends them
// only on an explicit operator request; the game clock's own day/night
// crossings do not send them.
const (
	// OpcodeSunRise is the wire opcode for SunRise, which turns the
	// client's sky to day.
	OpcodeSunRise = 0x1c
	// OpcodeSunSet is the wire opcode for SunSet, which turns the client's
	// sky to night.
	OpcodeSunSet = 0x1d
)

// OpcodeExRedSky is the extended sub-opcode for ExRedSky, which tints the
// client's sky red for a number of seconds.
const OpcodeExRedSky uint16 = 0x0040

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

// FrameExRedSky builds the ExRedSky packet, a red sky lasting duration
// seconds, as an owned frame.
func FrameExRedSky(duration int32) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExRedSky)
	w.WriteInt32(duration)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
