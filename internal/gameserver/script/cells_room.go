package script

import (
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// RoomCell is one room of a script's room set: an index, a lock with the
// game tick of its last change, and the party sent into the room with its
// members' object ids as they were when it was sent. Like the other cells
// it is leaf-locked and takes no closure; unlike them it is built by
// Script.NewRoom, which gives it the game tick to stamp.
//
// The room's party is named by identity and followed as it changes: its
// leader and members are read live, and a party since disbanded keeps the
// leader it last had and has no members. Asking a room that holds no party
// for its party's members or ids panics.
type RoomCell struct {
	index int
	tick  func() int32

	mu        sync.Mutex
	locked    bool
	time      int32
	party     player.Party
	memberIDs []int32
}

// NewRoom returns the unlocked room index, holding no party, whose lock
// changes are stamped with the script's game tick.
func (s *Script) NewRoom(index int) *RoomCell {
	return &RoomCell{index: index, tick: s.CurrentTick}
}

// Index returns the room's index.
func (r *RoomCell) Index() int { return r.index }

// Time returns the game tick of the room's last lock change, 0 before the
// first.
func (r *RoomCell) Time() int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.time
}

// Locked reports whether the room is locked.
func (r *RoomCell) Locked() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locked
}

// SetLock locks or unlocks the room and stamps the game tick, whether or
// not the lock changes.
func (r *RoomCell) SetLock(locked bool) {
	now := r.tick()
	r.mu.Lock()
	r.locked, r.time = locked, now
	r.mu.Unlock()
}

// TryLock locks an unlocked room, stamping the game tick, and reports
// whether it did: a lock check followed by the lock, in one
// step, so two parties never take one room. A locked room is left as it
// is.
func (r *RoomCell) TryLock() bool {
	now := r.tick()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locked {
		return false
	}
	r.locked, r.time = true, now
	return true
}

// SetParty makes the party p is in the room's, and takes the object ids of
// its members as they are now. A player in no party panics.
func (r *RoomCell) SetParty(p *Player) {
	ref, ok := p.character().Party()
	if !ok {
		panic("script: a room given the party of a player in no party")
	}
	r.setParty(ref)
}

// SetPartyFrom makes other's party the room's, and takes the object ids of
// its members as they are now, not as other took them. other holding no
// party panics.
func (r *RoomCell) SetPartyFrom(other *RoomCell) {
	r.setParty(other.mustParty())
}

func (r *RoomCell) setParty(ref player.Party) {
	members := ref.Members()
	ids := make([]int32, len(members))
	for i, m := range members {
		ids[i] = m.ObjectID()
	}
	r.mu.Lock()
	r.party, r.memberIDs = ref, ids
	r.mu.Unlock()
}

// heldParty returns the room's party, nil when it holds none.
func (r *RoomCell) heldParty() player.Party {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.party
}

// mustParty returns the room's party; a room holding none panics.
func (r *RoomCell) mustParty() player.Party {
	ref := r.heldParty()
	if ref == nil {
		panic("script: the party of a room holding none")
	}
	return ref
}

// HoldsPartyOf reports whether the room's party is the one p is in. A
// player in no party matches a room holding none, as an identity compare
// of two absent parties does.
func (r *RoomCell) HoldsPartyOf(p *Player) bool {
	ref, _ := p.character().Party()
	return r.heldParty() == ref
}

// PartyLeader returns the leader of the room's party, the last one it had
// once it disbanded; nil while that leader is not in the world, and for a
// room holding no party, which a script checks before asking whether a
// player leads the room's party.
func (r *RoomCell) PartyLeader() *Player {
	ref := r.heldParty()
	if ref == nil {
		return nil
	}
	return leaderOf(ref)
}

// PartyMembers returns the players of the room's party in the world, in
// the order they joined; empty once that party disbanded. A room holding
// no party panics.
func (r *RoomCell) PartyMembers() []*Player {
	return playersOf(r.mustParty().Members())
}

// MemberIDs returns the object ids of the members of the room's party as
// they were when it was set, in party order. A room holding no party
// panics.
func (r *RoomCell) MemberIDs() []int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.party == nil {
		panic("script: the member ids of a room holding no party")
	}
	return slices.Clone(r.memberIDs)
}

// PartyChanged reports whether the room's party is not the one it was
// sent in with: true when the room holds no party, when the party's member
// count differs from the ids taken, or when one of its members is not
// among them. A party since disbanded has no members, so it changed.
func (r *RoomCell) PartyChanged() bool {
	r.mu.Lock()
	ref, ids := r.party, r.memberIDs
	r.mu.Unlock()
	if ref == nil {
		return true
	}
	members := ref.Members()
	if len(members) != len(ids) {
		return true
	}
	for _, m := range members {
		if !slices.Contains(ids, m.ObjectID()) {
			return true
		}
	}
	return false
}

// Clear drops the room's party and its ids, and unlocks the room, stamping
// the game tick.
func (r *RoomCell) Clear() {
	now := r.tick()
	r.mu.Lock()
	r.party, r.memberIDs = nil, nil
	r.locked, r.time = false, now
	r.mu.Unlock()
}

// leaderOf returns the handle of ref's leader; nil while the leader is not
// in the world.
func leaderOf(ref player.Party) *Player {
	return playerOfCharacter(ref.Leader())
}
