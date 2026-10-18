package script

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestRoom returns room index of a script whose game tick reads *now
// seconds of uptime.
func newTestRoom(index int, now *atomic.Int64) *RoomCell {
	s := &Script{env: &Env{Uptime: func() time.Duration { return time.Duration(now.Load()) * time.Second }}}
	return s.NewRoom(index)
}

// A room starts unlocked at tick 0 holding no party; every lock change,
// a repeated one included, and a clear stamp the game tick of the call.
func TestRoomCellLockStampsTheTick(t *testing.T) {
	var now atomic.Int64
	r := newTestRoom(5, &now)
	if r.Index() != 5 || r.Locked() || r.Time() != 0 {
		t.Fatalf("new room: index %d locked %v time %d, want 5 false 0", r.Index(), r.Locked(), r.Time())
	}
	if !r.PartyChanged() {
		t.Fatal("a room holding no party reports its party unchanged")
	}

	now.Store(100)
	r.SetLock(true)
	if !r.Locked() || r.Time() != 100 {
		t.Fatalf("after SetLock(true) at 100: locked %v time %d", r.Locked(), r.Time())
	}
	now.Store(130)
	r.SetLock(true)
	if !r.Locked() || r.Time() != 130 {
		t.Fatalf("after a repeated SetLock(true) at 130: locked %v time %d", r.Locked(), r.Time())
	}
	now.Store(150)
	if r.TryLock() || r.Time() != 130 {
		t.Fatalf("TryLock on a locked room took it or stamped %d", r.Time())
	}
	now.Store(200)
	r.Clear()
	if r.Locked() || r.Time() != 200 || !r.PartyChanged() {
		t.Fatalf("after Clear at 200: locked %v time %d changed %v", r.Locked(), r.Time(), r.PartyChanged())
	}
	now.Store(210)
	if !r.TryLock() || !r.Locked() || r.Time() != 210 {
		t.Fatalf("TryLock on an unlocked room at 210: locked %v time %d", r.Locked(), r.Time())
	}
	now.Store(220)
	r.SetLock(false)
	if r.Locked() || r.Time() != 220 {
		t.Fatalf("after SetLock(false) at 220: locked %v time %d", r.Locked(), r.Time())
	}
}

// Of many concurrent TryLocks on one unlocked room exactly one wins.
func TestRoomCellTryLockHasOneWinner(t *testing.T) {
	var now atomic.Int64
	r := newTestRoom(0, &now)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if r.TryLock() {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d TryLocks won, want 1", wins.Load())
	}
}

// Reading the party, its leader, its members or its ids of a room holding
// none panics, and so does copying it into another room.
func TestRoomCellWithoutPartyPanics(t *testing.T) {
	var now atomic.Int64
	r := newTestRoom(0, &now)
	other := newTestRoom(1, &now)
	for name, fn := range map[string]func(){
		"PartyLeader":  func() { r.PartyLeader() },
		"PartyMembers": func() { r.PartyMembers() },
		"MemberIDs":    func() { r.MemberIDs() },
		"SetPartyFrom": func() { other.SetPartyFrom(r) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s on a room holding no party did not panic", name)
				}
			}()
			fn()
		}()
	}
}
