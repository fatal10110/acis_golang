package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// The tutorial window's opcodes.
const (
	OpcodeTutorialShowHTML          = 0xa0
	OpcodeTutorialShowQuestionMark  = 0xa1
	OpcodeTutorialEnableClientEvent = 0xa2
	OpcodeTutorialCloseHTML         = 0xa3
)

// FrameTutorialShowHTML opens the tutorial window on page.
func FrameTutorialShowHTML(page string) wire.Frame {
	w := newFrameWriter(OpcodeTutorialShowHTML)
	w.WriteString(page)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTutorialShowQuestionMark shows the tutorial question mark id, which
// the client answers with RequestTutorialQuestionMark once clicked.
func FrameTutorialShowQuestionMark(id int32) wire.Frame {
	w := newFrameWriter(OpcodeTutorialShowQuestionMark)
	w.WriteInt32(id)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTutorialEnableClientEvent makes the client report the tutorial
// client event id with RequestTutorialClientEvent when it happens.
func FrameTutorialEnableClientEvent(id int32) wire.Frame {
	w := newFrameWriter(OpcodeTutorialEnableClientEvent)
	w.WriteInt32(id)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTutorialCloseHTML closes the tutorial window.
func FrameTutorialCloseHTML() wire.Frame {
	w := newFrameWriter(OpcodeTutorialCloseHTML)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// OpcodeRadarControl is the wire opcode of RadarControl.
const OpcodeRadarControl = 0xeb

// Radar describes one RadarControl: Show is 0 to show a marker, 1 to
// remove it and 2 to clear the markers, and Type the marker's kind.
type Radar struct {
	Show, Type int32
	X, Y, Z    int32
}

// FrameRadarControl builds the RadarControl r.
func FrameRadarControl(r Radar) wire.Frame {
	w := newFrameWriter(OpcodeRadarControl)
	w.WriteInt32(r.Show)
	w.WriteInt32(r.Type)
	w.WriteInt32(r.X)
	w.WriteInt32(r.Y)
	w.WriteInt32(r.Z)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
