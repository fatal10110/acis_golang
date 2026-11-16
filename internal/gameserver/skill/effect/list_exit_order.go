package effect

import "slices"

// exitTicket is one exit hook of a stacked effect, decided under l.mu (the
// head a newcomer displaced, a buff an insertion retired, a removed head)
// and run later, outside it. An activation of the same stack group decided
// while the hook is still open starts only once the hook has run, so a late
// exit hook never clears owner state the later head's start set.
//
// Whoever claims the ticket first runs the hook: the call that decided it,
// at its usual place among that call's hooks, or the activation run (see
// runActivations), which takes over a hook not yet started ahead of the
// start that must follow it. A hook another goroutine is already running
// is never waited for: the activation run hands its queue over to that
// goroutine, which resumes it once the hook returns.
type exitTicket struct {
	group string
	run   func()
	// state and parked are read and written under the list's mu.
	state exitState
	// parked records that the activation run stopped at this ticket and
	// handed its queue over to the goroutine running it.
	parked bool
}

type exitState uint8

const (
	exitQueued exitState = iota
	exitRunning
	exitDone
)

// orderExit registers thunk, the exit hook of stacked effect e that the
// caller decided, with e's stack group and returns what the caller runs in
// its place: the hook, unless an activation already took it over. A nil
// thunk, or one of an unstacked effect, is returned unchanged. Caller must
// hold l.mu.
func (l *List) orderExit(e *Effect, thunk func()) func() {
	if thunk == nil || e.stackType() == "none" {
		return thunk
	}
	t := &exitTicket{group: e.stackType(), run: thunk}
	if l.openExits == nil {
		l.openExits = make(map[string][]*exitTicket)
	}
	l.openExits[t.group] = append(l.openExits[t.group], t)
	return func() {
		l.mu.Lock()
		if t.state != exitQueued {
			l.mu.Unlock()
			return
		}
		t.state = exitRunning
		l.mu.Unlock()
		l.runExit(t)
	}
}

// runExit runs t's hook, which the caller claimed, closes t, and resumes the
// activations parked on it. A hook that panics still closes t; the parked
// activations then run ahead of the next one queued.
func (l *List) runExit(t *exitTicket) {
	ran := false
	defer func() {
		if !ran {
			l.mu.Lock()
			l.closeExitLocked(t)
			l.mu.Unlock()
		}
	}()
	t.run()
	ran = true

	l.mu.Lock()
	l.closeExitLocked(t)
	if t.parked {
		l.runActivations()
		return
	}
	l.mu.Unlock()
}

// closeExitLocked marks t done and drops it from its group's open exits.
// Caller must hold l.mu.
func (l *List) closeExitLocked(t *exitTicket) {
	t.state = exitDone
	open := slices.DeleteFunc(l.openExits[t.group], func(o *exitTicket) bool { return o == t })
	if len(open) == 0 {
		delete(l.openExits, t.group)
		return
	}
	l.openExits[t.group] = open
}

// exitsBeforeLocked returns the exit hooks of e's stack group still open,
// which an activation of e decided now must let finish before it starts.
// Caller must hold l.mu.
func (l *List) exitsBeforeLocked(e *Effect) []*exitTicket {
	if e.stackType() == "none" {
		return nil
	}
	return slices.Clone(l.openExits[e.stackType()])
}

// openExit returns the first exit hook act must let finish that has not.
// Caller must hold l.mu.
func (act activation) openExit() *exitTicket {
	for _, t := range act.after {
		if t.state != exitDone {
			return t
		}
	}
	return nil
}
