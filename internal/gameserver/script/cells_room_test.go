package script

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
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

// A room holding no party reads no members, no ids and no leader, and has
// none to hand over (#5170).
func TestRoomCellWithoutPartyReadsNothing(t *testing.T) {
	var now atomic.Int64
	r := newTestRoom(0, &now)
	other := newTestRoom(1, &now)
	if r.PartyLeader() != nil || r.PartyMembers() != nil || r.MemberIDs() != nil {
		t.Error("a room holding no party reads a party")
	}
	if r.MovePartyTo(other) || other.Locked() || other.heldParty() != nil {
		t.Error("a room holding no party handed one over")
	}
}

// roomParty is a party of no members, compared by identity.
type roomParty struct{ name string }

func (*roomParty) Leader() *player.Character    { return nil }
func (*roomParty) Members() []*player.Character { return nil }

// A hand-over locks the target holding the party, stamped, and clears and
// unlocks the source; a locked target or the room itself takes nothing.
func TestRoomCellMovePartyTo(t *testing.T) {
	var now atomic.Int64
	from, to, busy := newTestRoom(4, &now), newTestRoom(2, &now), newTestRoom(7, &now)
	party := &roomParty{"P"}
	from.setParty(party)
	from.SetLock(true)
	busy.SetLock(true)

	now.Store(50)
	if from.MovePartyTo(busy) || from.MovePartyTo(from) {
		t.Fatal("a hand-over to a locked room or to the room itself moved the party")
	}
	if !from.Locked() || from.heldParty() != party || busy.heldParty() != nil || from.Time() != 0 {
		t.Fatal("a failed hand-over changed a room")
	}
	if !from.MovePartyTo(to) {
		t.Fatal("the hand-over to an unlocked room failed")
	}
	if !to.Locked() || to.heldParty() != party || to.Time() != 50 {
		t.Errorf("target: locked %v holds %v time %d, want locked holding P at 50", to.Locked(), to.heldParty(), to.Time())
	}
	if from.Locked() || from.heldParty() != nil || from.Time() != 50 {
		t.Errorf("source: locked %v holds %v time %d, want cleared at 50", from.Locked(), from.heldParty(), from.Time())
	}
}

// Hand-overs of one room to two rooms racing a clear of it (a give-up)
// leave the party in at most one room, and no room locked without a party
// (#5170). Opposite hand-overs between two rooms do not deadlock.
func TestRoomCellMovePartyToRaces(t *testing.T) {
	var now atomic.Int64
	for range 500 {
		src, a, b := newTestRoom(3, &now), newTestRoom(1, &now), newTestRoom(5, &now)
		party := &roomParty{"P"}
		src.setParty(party)
		src.SetLock(true)
		var wg sync.WaitGroup
		wg.Go(func() { src.MovePartyTo(a) })
		wg.Go(func() { src.MovePartyTo(b) })
		wg.Go(func() { src.Clear() })
		wg.Go(func() { a.MovePartyTo(b) })
		wg.Go(func() { b.MovePartyTo(a) })
		wg.Wait()
		held := 0
		for _, r := range []*RoomCell{src, a, b} {
			p := r.heldParty()
			if p != nil {
				held++
			}
			if r.Locked() && p == nil {
				t.Fatalf("room %d is locked without a party", r.Index())
			}
		}
		if held > 1 {
			t.Fatalf("the party is booked in %d rooms", held)
		}
	}
}
