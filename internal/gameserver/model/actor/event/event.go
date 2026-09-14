// Package event is the closed set of facts an actor reports to whatever
// delivers them outside the domain. Domain code emits an Event describing
// what happened; the network layer maps each one to packets in a single type
// switch per actor kind.
package event

import "sync"

// Event is one thing that happened to an actor which something outside the
// domain must react to. The set is closed: every type lives in this package.
type Event interface{ event() }

// Sink receives an actor's events. Implementations must be safe to call from
// any goroutine and must not block.
type Sink interface{ Emit(Event) }

// Object is the identity an event carries when the receiver needs the world
// object itself rather than only its id.
type Object interface{ ObjectID() int32 }

// Recorder is the test Sink: it appends every event under a mutex.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Emit records e.
func (r *Recorder) Emit(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// Events returns a copy of every event recorded so far, in emit order.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// Count returns how many recorded events have type T.
func Count[T Event](r *Recorder) int {
	n := 0
	for _, e := range r.Events() {
		if _, ok := e.(T); ok {
			n++
		}
	}
	return n
}

// Of returns every recorded event of type T, in emit order.
func Of[T Event](r *Recorder) []T {
	var out []T
	for _, e := range r.Events() {
		if t, ok := e.(T); ok {
			out = append(out, t)
		}
	}
	return out
}
