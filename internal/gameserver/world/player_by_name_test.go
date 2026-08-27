package world

import (
	"sync/atomic"
	"testing"
	"time"
)

type namedPlayerObject struct {
	id   int32
	name string
}

func (o namedPlayerObject) ObjectID() int32       { return o.id }
func (o namedPlayerObject) CharacterName() string { return o.name }

// blockingNamedPlayer's CharacterName blocks on its first call (the one
// AddPlayer makes) until ready is closed, and returns immediately on every
// later call (the one RemovePlayer makes). That pins AddPlayer mid-call so a
// concurrently issued RemovePlayer for the same id can be forced to run
// its full critical section first, deterministically reproducing the
// interleave a non-atomic add/remove would allow.
type blockingNamedPlayer struct {
	id      int32
	name    string
	ready   chan struct{}
	started chan struct{}
	calls   int32
}

func (o *blockingNamedPlayer) ObjectID() int32 { return o.id }

func (o *blockingNamedPlayer) CharacterName() string {
	if atomic.AddInt32(&o.calls, 1) == 1 {
		close(o.started)
		<-o.ready
	}
	return o.name
}

func TestState_PlayerByName(t *testing.T) {
	s := New()
	p := namedPlayerObject{id: 42, name: "Newbie"}
	s.AddPlayer(p)

	t.Run("found, case-insensitive", func(t *testing.T) {
		got, ok := s.PlayerByName("newBIE")
		if !ok {
			t.Fatal("PlayerByName() ok = false, want true")
		}
		if got.ObjectID() != p.id {
			t.Errorf("PlayerByName() = %v, want %v", got.ObjectID(), p.id)
		}
	})

	t.Run("missing", func(t *testing.T) {
		if _, ok := s.PlayerByName("Ghost"); ok {
			t.Error("PlayerByName(\"Ghost\") ok = true, want false")
		}
	})

	t.Run("removed", func(t *testing.T) {
		s.RemovePlayer(p.id)
		if _, ok := s.PlayerByName("Newbie"); ok {
			t.Error("PlayerByName() after RemovePlayer ok = true, want false")
		}
	})
}

// TestState_AddRemovePlayer_ConcurrentSameID_NoStaleName forces AddPlayer
// and RemovePlayer for the same id to interleave: AddPlayer is pinned mid-
// call (via blockingNamedPlayer) after registering in the players registry
// but before it can touch the name index, RemovePlayer is then run to
// completion for that same id, and only after that does AddPlayer resume.
// A non-atomic registry/name-index update lets RemovePlayer's delete-guard
// see no name entry yet (so it no-ops) and then lets AddPlayer's
// first-wins guard write a name entry for an id that is no longer online —
// a stale entry that permanently blocks a later, different player from
// claiming that name. Fixed code makes the whole add/remove one critical
// section, so this exact interleave cannot happen.
func TestState_AddRemovePlayer_ConcurrentSameID_NoStaleName(t *testing.T) {
	s := New()
	p := &blockingNamedPlayer{id: 1, name: "Newbie", ready: make(chan struct{}), started: make(chan struct{})}

	addDone := make(chan struct{})
	go func() {
		s.AddPlayer(p)
		close(addDone)
	}()

	<-p.started // AddPlayer is now blocked inside its CharacterName call

	removeDone := make(chan struct{})
	go func() {
		s.RemovePlayer(p.id)
		close(removeDone)
	}()

	select {
	case <-removeDone:
		// Old, buggy code: RemovePlayer isn't blocked by AddPlayer's
		// in-flight call, so it runs to completion here.
	case <-time.After(200 * time.Millisecond):
		// Fixed code: RemovePlayer blocks on the same lock AddPlayer
		// holds for its whole call, so it can't finish yet. Either way,
		// unblock AddPlayer and let both settle.
	}

	close(p.ready)
	<-addDone
	<-removeDone

	other := namedPlayerObject{id: 2, name: "Newbie"}
	s.AddPlayer(other)

	got, ok := s.PlayerByName("Newbie")
	if !ok || got.ObjectID() != other.id {
		t.Fatalf("PlayerByName(\"Newbie\") = %v, %v; want id %d — a stale name-index entry from id 1 blocked reuse", got, ok, other.id)
	}
}
