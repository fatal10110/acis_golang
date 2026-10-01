package partymatch

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
)

// fake is a Member whose departure the test controls and whose room
// records are kept.
type fake struct {
	id       int32
	level    int
	departed atomic.Bool

	mu    sync.Mutex
	room  int32
	rooms []int32 // every SetPartyRoom value, in order
}

func newFake(id int32) *fake { return &fake{id: id, level: 40} }

func (f *fake) ObjectID() int32       { return f.id }
func (f *fake) Level() int            { return f.level }
func (f *fake) CharacterName() string { return "p" }
func (f *fake) Departed() bool        { return f.departed.Load() }

func (f *fake) SetPartyRoom(id int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.room = id
	f.rooms = append(f.rooms, id)
}

func (f *fake) partyRoom() int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.room
}

func (f *fake) roomHistory() []int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rooms)
}

var anyRoom = Settings{MaxMembers: 12, MinLevel: 1, MaxLevel: 80, Title: "t"}

func noParty(_, _ *fake) bool { return false }

// openRoom opens a room led by a new waiting player and returns its id.
func openRoom(t *testing.T, r *Registry[*fake], leader *fake) int32 {
	t.Helper()
	r.AddWaiting(leader)
	if out := r.Open(leader, anyRoom, 1, nil); len(out) == 0 {
		t.Fatalf("open by %d: no notices", leader.id)
	}
	rm, ok := r.RoomOf(leader.id)
	if !ok {
		t.Fatalf("open by %d: no room", leader.id)
	}
	return rm.ID
}

// assertOut checks p is in no room and not on the waiting list, and that
// its room flag is 0.
func assertOut(t *testing.T, r *Registry[*fake], p *fake) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if rm := r.byMember[p.id]; rm != nil {
		t.Errorf("player %d mapped to room %d", p.id, rm.id)
	}
	for _, m := range r.waiting {
		if m.id == p.id {
			t.Errorf("player %d on the waiting list", p.id)
		}
	}
	for _, rm := range r.rooms {
		if rm.contains(p) {
			t.Errorf("room %d lists player %d", rm.id, p.id)
		}
	}
	if got := p.partyRoom(); got != 0 {
		t.Errorf("player %d room flag = %d, want 0", p.id, got)
	}
}

func members(t *testing.T, r *Registry[*fake], roomID int32) []int32 {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	rm := r.rooms[roomID]
	if rm == nil {
		t.Fatalf("room %d gone", roomID)
	}
	var ids []int32
	for _, m := range rm.members {
		ids = append(ids, m.id)
	}
	return ids
}

func onWaiting(r *Registry[*fake], p *fake) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.ContainsFunc(r.waiting, func(m *fake) bool { return m.id == p.id })
}

func TestOpenLeavesDepartedPartyMemberOut(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, gone, mate := newFake(1), newFake(2), newFake(3)
	gone.departed.Store(true)
	r.AddWaiting(leader)

	r.Open(leader, anyRoom, 1, []*fake{leader, gone, mate})

	rm, ok := r.RoomOf(leader.id)
	if !ok {
		t.Fatal("no room opened")
	}
	if got := members(t, r, rm.ID); !slices.Equal(got, []int32{1, 3}) {
		t.Errorf("members = %v, want [1 3]", got)
	}
	assertOut(t, r, gone)
	if h := gone.roomHistory(); len(h) != 0 {
		t.Errorf("departed member's room set to %v", h)
	}
	if mate.partyRoom() != rm.ID {
		t.Errorf("mate room flag = %d, want %d", mate.partyRoom(), rm.ID)
	}
}

func TestOpenByDepartedLeaderOpensNothing(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, mate := newFake(1), newFake(2)
	r.AddWaiting(leader)
	leader.departed.Store(true)

	if out := r.Open(leader, anyRoom, 1, []*fake{leader, mate}); out != nil {
		t.Errorf("open notices = %v, want none", out)
	}
	r.Leave(leader)
	assertOut(t, r, leader)
	assertOut(t, r, mate)
	if len(r.rooms) != 0 {
		t.Errorf("%d rooms left", len(r.rooms))
	}
}

func TestOpenLeavesMemberOfAnotherRoomWhereItIs(t *testing.T) {
	r := NewRegistry[*fake]()
	other, mate, leader := newFake(1), newFake(2), newFake(3)
	otherID := openRoom(t, r, other)
	r.AddWaiting(mate)
	if _, st := r.Join(mate, otherID, AnyLocation, 1, nil); st != Joined {
		t.Fatalf("join = %v", st)
	}
	r.AddWaiting(leader)

	r.Open(leader, anyRoom, 1, []*fake{leader, mate})

	rm, _ := r.RoomOf(leader.id)
	if got := members(t, r, rm.ID); !slices.Equal(got, []int32{3}) {
		t.Errorf("new room members = %v, want [3]", got)
	}
	if got := members(t, r, otherID); !slices.Equal(got, []int32{1, 2}) {
		t.Errorf("other room members = %v, want [1 2]", got)
	}
	if own, _ := r.RoomOf(mate.id); own.ID != otherID {
		t.Errorf("mate in room %d, want %d", own.ID, otherID)
	}
	if h := mate.roomHistory(); !slices.Equal(h, []int32{otherID}) {
		t.Errorf("mate room flags = %v, want [%d]", h, otherID)
	}
}

func TestJoinRefusesDepartedPlayer(t *testing.T) {
	for _, viaAnswer := range []bool{false, true} {
		r := NewRegistry[*fake]()
		leader, p := newFake(1), newFake(2)
		id := openRoom(t, r, leader)
		r.AddWaiting(p)
		p.departed.Store(true)

		var st JoinStatus
		var out []Notice
		if viaAnswer {
			out, st = r.Answer(p, leader)
		} else {
			out, st = r.Join(p, id, AnyLocation, 1, nil)
		}
		if st != JoinNotWaiting || out != nil {
			t.Errorf("answer=%v: status %v, %d notices; want JoinNotWaiting, none", viaAnswer, st, len(out))
		}
		r.Leave(p)
		assertOut(t, r, p)
		if got := members(t, r, id); !slices.Equal(got, []int32{1}) {
			t.Errorf("answer=%v: members = %v, want [1]", viaAnswer, got)
		}
	}
}

func TestJoinRefusesMemberOfAnotherRoom(t *testing.T) {
	r := NewRegistry[*fake]()
	a, b, p := newFake(1), newFake(2), newFake(3)
	aID := openRoom(t, r, a)
	bID := openRoom(t, r, b)
	r.AddWaiting(p)
	if _, st := r.Join(p, aID, AnyLocation, 1, nil); st != Joined {
		t.Fatalf("first join = %v", st)
	}
	if onWaiting(r, p) {
		t.Error("room member still on the waiting list")
	}

	r.AddWaiting(p) // a room member is not listed
	if onWaiting(r, p) {
		t.Error("AddWaiting listed a room member")
	}
	if _, st := r.Join(p, bID, AnyLocation, 1, nil); st != JoinNotWaiting {
		t.Errorf("second join = %v, want JoinNotWaiting", st)
	}
	if got := members(t, r, bID); !slices.Equal(got, []int32{2}) {
		t.Errorf("room b members = %v, want [2]", got)
	}
	if h := p.roomHistory(); !slices.Equal(h, []int32{aID}) {
		t.Errorf("room flags = %v, want [%d]", h, aID)
	}
}

func TestPartyJoinedRefusesDepartedPlayer(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, p := newFake(1), newFake(2)
	id := openRoom(t, r, leader)
	r.AddWaiting(p)
	p.departed.Store(true)

	if out := r.PartyJoined(leader, p); out != nil {
		t.Errorf("notices = %v, want none", out)
	}
	r.Leave(p)
	assertOut(t, r, p)
	if h := p.roomHistory(); len(h) != 0 {
		t.Errorf("room flags = %v, want none", h)
	}
	if got := members(t, r, id); !slices.Equal(got, []int32{1}) {
		t.Errorf("members = %v, want [1]", got)
	}
}

func TestPartyJoinedLeavesMemberOfAnotherRoomWhereItIs(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, other, p := newFake(1), newFake(2), newFake(3)
	id := openRoom(t, r, leader)
	otherID := openRoom(t, r, other)
	r.AddWaiting(p)
	r.Join(p, otherID, AnyLocation, 1, nil)

	if out := r.PartyJoined(leader, p); out != nil {
		t.Errorf("notices = %v, want none", out)
	}
	if got := members(t, r, id); !slices.Equal(got, []int32{1}) {
		t.Errorf("inviter's room members = %v, want [1]", got)
	}
	if own, _ := r.RoomOf(p.id); own.ID != otherID {
		t.Errorf("player in room %d, want %d", own.ID, otherID)
	}
	if h := p.roomHistory(); !slices.Equal(h, []int32{otherID}) {
		t.Errorf("room flags = %v, want [%d]", h, otherID)
	}
}

func TestPartyJoinedTakesJoinerOffWaitingList(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, p := newFake(1), newFake(2)
	id := openRoom(t, r, leader)
	r.AddWaiting(p)

	r.PartyJoined(leader, p)

	if onWaiting(r, p) {
		t.Error("joiner still on the waiting list")
	}
	if got := members(t, r, id); !slices.Equal(got, []int32{1, 2}) {
		t.Errorf("members = %v, want [1 2]", got)
	}
	if p.partyRoom() != id {
		t.Errorf("room flag = %d, want %d", p.partyRoom(), id)
	}
}

func TestAddWaitingRefusesDepartedPlayer(t *testing.T) {
	r := NewRegistry[*fake]()
	p := newFake(1)
	p.departed.Store(true)
	r.AddWaiting(p)
	assertOut(t, r, p)
}

func TestOustDoesNotRelistDepartedTarget(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, p := newFake(1), newFake(2)
	id := openRoom(t, r, leader)
	r.AddWaiting(p)
	r.Join(p, id, AnyLocation, 1, nil)
	p.departed.Store(true)

	if _, st := r.Oust(leader, p, noParty); st != Ousted {
		t.Fatalf("oust = %v", st)
	}
	assertOut(t, r, p)
	if out := r.Leave(p); out != nil {
		t.Errorf("leave after oust: %d notices, want none", len(out))
	}
	assertOut(t, r, p)
}

func TestOustRelistsTarget(t *testing.T) {
	r := NewRegistry[*fake]()
	leader, p := newFake(1), newFake(2)
	id := openRoom(t, r, leader)
	r.AddWaiting(p)
	r.Join(p, id, AnyLocation, 1, nil)

	r.Oust(leader, p, noParty)

	if !onWaiting(r, p) {
		t.Error("ousted target not back on the waiting list")
	}
	if p.partyRoom() != 0 {
		t.Errorf("room flag = %d, want 0", p.partyRoom())
	}
}

// TestAddAfterLeaveLeavesNothing drives each add path after a departing
// player's Leave has run: none may put the player back.
func TestAddAfterLeaveLeavesNothing(t *testing.T) {
	adds := map[string]func(r *Registry[*fake], leader, p *fake, id int32){
		"AddWaiting": func(r *Registry[*fake], _, p *fake, _ int32) { r.AddWaiting(p) },
		"Join":       func(r *Registry[*fake], _, p *fake, id int32) { r.Join(p, id, AnyLocation, 1, nil) },
		"JoinAuto":   func(r *Registry[*fake], _, p *fake, _ int32) { r.Join(p, 0, AnyLocation, 1, nil) },
		"Answer":     func(r *Registry[*fake], leader, p *fake, _ int32) { r.Answer(p, leader) },
		"PartyJoined": func(r *Registry[*fake], leader, p *fake, _ int32) {
			r.PartyJoined(leader, p)
		},
		"OpenAsMate": func(r *Registry[*fake], _, p *fake, _ int32) {
			second := newFake(9)
			r.AddWaiting(second)
			r.Open(second, anyRoom, 1, []*fake{second, p})
		},
		"OpenAsLeader": func(r *Registry[*fake], _, p *fake, _ int32) {
			r.Open(p, anyRoom, 1, nil)
		},
	}
	for name, add := range adds {
		t.Run(name, func(t *testing.T) {
			r := NewRegistry[*fake]()
			leader, p := newFake(1), newFake(2)
			id := openRoom(t, r, leader)
			r.AddWaiting(p)

			p.departed.Store(true)
			r.Leave(p)
			add(r, leader, p, id)

			assertOut(t, r, p)
			if h := p.roomHistory(); len(h) != 0 {
				t.Errorf("room flags = %v, want none", h)
			}
		})
	}
}

// TestAddRacingLeaveLeavesNothing races each add path against a player's
// departure (mark, then Leave, as detach does): whichever runs first under
// the registry lock, the player ends out of every room and off the list.
func TestAddRacingLeaveLeavesNothing(t *testing.T) {
	adds := map[string]func(r *Registry[*fake], leader, p *fake, id int32){
		"AddWaiting":  func(r *Registry[*fake], _, p *fake, _ int32) { r.AddWaiting(p) },
		"Join":        func(r *Registry[*fake], _, p *fake, id int32) { r.Join(p, id, AnyLocation, 1, nil) },
		"PartyJoined": func(r *Registry[*fake], leader, p *fake, _ int32) { r.PartyJoined(leader, p) },
		"OpenAsMate": func(r *Registry[*fake], leader, p *fake, _ int32) {
			r.Open(leader, anyRoom, 1, []*fake{leader, p})
		},
	}
	for name, add := range adds {
		t.Run(name, func(t *testing.T) {
			for range 200 {
				r := NewRegistry[*fake]()
				leader, p := newFake(1), newFake(2)
				id := int32(0)
				if name == "OpenAsMate" {
					r.AddWaiting(leader)
				} else {
					id = openRoom(t, r, leader)
				}
				r.AddWaiting(p)

				var wg sync.WaitGroup
				wg.Go(func() { add(r, leader, p, id) })
				wg.Go(func() {
					p.departed.Store(true)
					r.Leave(p)
				})
				wg.Wait()

				assertOut(t, r, p)
				if t.Failed() {
					return
				}
			}
		})
	}
}
