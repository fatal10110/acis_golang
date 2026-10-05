package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// frameReceiver is a broadcast recipient: it takes ownership of one frame
// copy made for it alone.
type frameReceiver interface {
	BroadcastFrame(wire.Frame) bool
}

// broadcastFrame builds one frame, only once recipients names a first
// receiver, and hands every receiver its own copy.
func broadcastFrame(build func() wire.Frame, recipients func(func(frameReceiver))) {
	f := newFanout(build)
	defer f.release()
	recipients(f.send)
}

// broadcastBuiltFrame hands every receiver recipients names its own copy of
// an already-built frame, taking ownership of frame.
func broadcastBuiltFrame(frame wire.Frame, recipients func(func(frameReceiver))) {
	f := fanout{frame: frame, built: true, rejects: outboundRejects}
	defer f.release()
	recipients(f.send)
}

// fanout is the one broadcast fan-out: it builds a frame on first use and
// hands each recipient an independent pooled copy, because a session
// encrypts its outgoing bytes in place. A frame no copy can be made of is
// reported once, with the copy's cause, and reaches no recipient.
type fanout struct {
	build   func() wire.Frame
	rejects *frameRejectReporter

	frame  wire.Frame
	built  bool
	failed bool
}

func newFanout(build func() wire.Frame) fanout {
	return fanout{build: build, rejects: outboundRejects}
}

// send hands receiver its own copy of the frame, building the frame first if
// no earlier recipient has.
func (f *fanout) send(receiver frameReceiver) {
	if f.failed {
		return
	}
	if !f.built {
		f.frame = f.build()
		f.built = true
	}
	frame, err := serverpackets.CopyFrame(f.frame)
	if err != nil {
		// Every later copy of the same source would fail the same way, so
		// the rejection is reported once per broadcast, not per recipient.
		f.failed = true
		f.rejects.report("broadcast", err)
		return
	}
	receiver.BroadcastFrame(frame)
}

// release returns the source frame's storage, if it was ever built.
func (f *fanout) release() {
	if f.built {
		f.frame.Release()
	}
}
