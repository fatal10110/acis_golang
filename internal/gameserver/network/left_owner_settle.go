package network

import "sync"

// selectingOwners holds back the offline settles of a pet corpse whose owner
// a character selection is restoring. A corpse its owner left behind settles
// with the owner's rows while the owner is out of the world: its items become
// the owner's rows, its collar row is deleted. A selection of that owner reads
// those rows before it registers its player, so a settle landing in between
// would change rows the selection has already loaded, leaving the session with
// a collar whose row is gone and without the corpse's items. The reference
// relinks the pet on the same restore that loads the inventory
// (Player.restore, Player.java:4144-4150), so no decay settles in between.
//
// A selection is under way from before it waits for the character's queued
// saves until it has registered its player and taken its pet over, or given
// up. A settle meanwhile is held and runs again as the selection ends, against
// the session it registered, or offline when it registered none. A settle that
// runs offline does so under the same lock a selection begins under, so its
// writes are queued either before the selection waits for the character's
// saves or not at all.
type selectingOwners struct {
	mu      sync.Mutex
	pending map[int32]*ownerSelection
}

// ownerSelection is the selections of one character under way and the
// settles held for them.
type ownerSelection struct {
	selections int
	held       []func()
}

// begin marks a selection of ownerID under way.
func (s *selectingOwners) begin(ownerID int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[int32]*ownerSelection)
	}
	sel, ok := s.pending[ownerID]
	if !ok {
		sel = &ownerSelection{}
		s.pending[ownerID] = sel
	}
	sel.selections++
}

// end closes a selection of ownerID that begin opened. The last one to end
// runs the settles held for it, in the order they were held, on the calling
// goroutine.
func (s *selectingOwners) end(ownerID int32) {
	s.mu.Lock()
	sel, ok := s.pending[ownerID]
	if !ok {
		s.mu.Unlock()
		return
	}
	sel.selections--
	if sel.selections > 0 {
		s.mu.Unlock()
		return
	}
	delete(s.pending, ownerID)
	held := sel.held
	s.mu.Unlock()
	for _, settle := range held {
		settle()
	}
}

// settle runs offline for ownerID unless a selection of ownerID is under
// way, which holds retry until it ends, or lookup, when given, finds ownerID
// in the world: settle then returns that session and leaves the settle to the
// caller. It reports whether it returned a session.
func (s *selectingOwners) settle(ownerID int32, lookup func(int32) (*livePlayer, bool), offline, retry func()) (*livePlayer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sel, ok := s.pending[ownerID]; ok {
		sel.held = append(sel.held, retry)
		return nil, false
	}
	if lookup != nil {
		if owner, ok := lookup(ownerID); ok {
			return owner, true
		}
	}
	offline()
	return nil, false
}

// settleWithLeftOwner settles part of a pet corpse ownerID left behind: with
// the session the owner came back with, on that session's queue, through
// online, or offline while the owner is out of the world. A session on its way
// out has queued its inventory's last writes by the time it is marked detached
// or its queue refuses this: the owner is offline then. A selection of the
// owner under way holds the settle until it ends (selectingOwners).
func (l *GameClientLink) settleWithLeftOwner(ownerID int32, online func(*livePlayer), offline func()) {
	retry := func() { l.settleWithLeftOwner(ownerID, online, offline) }
	owner, ok := l.selections.settle(ownerID, l.livePlayerByID, offline, retry)
	if !ok {
		return
	}
	leaving := func() { l.selections.settle(ownerID, nil, offline, retry) }
	posted := postLive(owner, func() {
		if owner.detached() {
			leaving()
			return
		}
		online(owner)
	})
	if !posted {
		leaving()
	}
}
