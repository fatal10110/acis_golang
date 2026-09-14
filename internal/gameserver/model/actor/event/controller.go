package event

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

// CastAborted reports an in-flight cast aborted; Interrupted means the abort
// went through the window-gated interrupt path.
type CastAborted struct{ Interrupted bool }

// CastFinished reports an in-flight cast ending, Interrupted by an abort or
// completed naturally.
type CastFinished struct {
	Interrupted bool
	Skill       modelskill.Definition
}

// CastStopAck reports a cast stop request, whether or not a cast was in
// flight.
type CastStopAck struct{}

// AttackStarted reports an attack animation starting, before its hits are
// scheduled.
type AttackStarted struct{}

// AttackFinished reports an attack animation finishing; the actor is free to
// act again.
type AttackFinished struct{}

// Arrived reports that movement a controller started reached its
// destination.
type Arrived struct{}

// MoveBlocked reports an in-flight move stopped by a newly blocked geodata
// path. The receiver owes observers the stopped-cell correction.
type MoveBlocked struct{}

func (CastAborted) event()    {}
func (CastFinished) event()   {}
func (CastStopAck) event()    {}
func (AttackStarted) event()  {}
func (AttackFinished) event() {}
func (Arrived) event()        {}
func (MoveBlocked) event()    {}
