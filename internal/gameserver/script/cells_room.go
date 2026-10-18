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
// leader and members are read live, and a party since disbanded reads as
// having no leader and no members. Asking a room that holds no party for
// its party's leader, members or ids panics, as the reference throws there.
type RoomCell struct {
	index int
	tick  func() int32

	mu        sync.Mutex
	locked    bool
	time      int32
	party     player.PartyRef
	hasParty  bool
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
// whether it did: the reference's lock check followed by its lock, in one
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
	r.setParty(other.heldParty())
}

func (r *RoomCell) setParty(ref player.PartyRef) {
	members := ref.Members()
	ids := make([]int32, len(members))
	for i, m := range members {
		ids[i] = m.ObjectID()
	}
	r.mu.Lock()
	r.party, r.hasParty, r.memberIDs = ref, true, ids
	r.mu.Unlock()
}

// heldParty returns the room's party; a room holding none panics.
func (r *RoomCell) heldParty() player.PartyRef {
	r.mu.Lock()
	ref, ok := r.party, r.hasParty
	r.mu.Unlock()
	if !ok {
		panic("script: the party of a room holding none")
	}
	return ref
}

// HoldsPartyOf reports whether the room's party is the one p is in. A
// player in no party matches a room holding none, as the reference's
// identity compare of two absent parties does.
func (r *RoomCell) HoldsPartyOf(p *Player) bool {
	ref, inParty := p.character().Party()
	r.mu.Lock()
	held, hasParty := r.party, r.hasParty
	r.mu.Unlock()
	if !inParty || !hasParty {
		return inParty == hasParty
	}
	return held.Same(ref)
}

// PartyLeader returns the leader of the room's party; nil once that party
// is gone or while its leader is not in the world. A room holding no party
// panics.
func (r *RoomCell) PartyLeader() *Player {
	return leaderOf(r.heldParty())
}

// PartyMembers returns the players of the room's party in the world, in
// the order they joined; empty once that party is gone. A room holding no
// party panics.
func (r *RoomCell) PartyMembers() []*Player {
	return playersOf(r.heldParty().Members())
}

// MemberIDs returns the object ids of the members of the room's party as
// they were when it was set, in party order. A room holding no party
// panics.
func (r *RoomCell) MemberIDs() []int32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasParty {
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
	ref, hasParty, ids := r.party, r.hasParty, r.memberIDs
	r.mu.Unlock()
	if !hasParty {
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
	r.party, r.hasParty, r.memberIDs = player.PartyRef{}, false, nil
	r.locked, r.time = false, now
	r.mu.Unlock()
}

// leaderOf returns the handle of ref's leader; nil once the party is gone
// or while the leader is not in the world.
func leaderOf(ref player.PartyRef) *Player {
	if leader := ref.Leader(); leader != nil {
		return playerOfCharacter(leader)
	}
	return nil
}
