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
//
// The held settles run in the order they were held, and the character's entry
// stays until they all have: a settle arriving while they run is held behind
// them, so the corpse's items still reach the owner ahead of its collar's
// delete.
type selectingOwners struct {
	mu      sync.Mutex
	pending map[int32]*ownerSelection
}

// ownerSelection is the selections of one character under way, the settles
// held for them, and whether the last selection to end is running those.
type ownerSelection struct {
	selections int
	draining   bool
	held       []func()
}

// begin marks a selection of ownerID under way. A selection beginning while
// the held settles of an earlier one run holds the rest of them for itself.
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
// goroutine, including those held while it runs them. It stops when a new
// selection of ownerID begins, which then runs the rest as it ends.
func (s *selectingOwners) end(ownerID int32) {
	s.mu.Lock()
	sel, ok := s.pending[ownerID]
	if !ok {
		s.mu.Unlock()
		return
	}
	sel.selections--
	if sel.selections > 0 || sel.draining {
		s.mu.Unlock()
		return
	}
	sel.draining = true
	for {
		if sel.selections > 0 {
			sel.draining = false
			s.mu.Unlock()
			return
		}
		if len(sel.held) == 0 {
			delete(s.pending, ownerID)
			s.mu.Unlock()
			return
		}
		next := sel.held[0]
		sel.held = sel.held[1:]
		s.mu.Unlock()
		next()
		s.mu.Lock()
	}
}

// settle runs offline for ownerID unless a selection of ownerID is under
// way, or its held settles are running, which holds retry until they have
// run, or lookup, when given, finds ownerID in the world: settle then returns
// that session and leaves the settle to the caller. It reports whether it
// returned a session.
//
// draining marks a held settle that end is running again: it goes ahead of
// the settles held behind it, unless a new selection has begun, which holds it
// again at the front.
func (s *selectingOwners) settle(ownerID int32, lookup func(int32) (*livePlayer, bool), offline, retry func(), draining bool) (*livePlayer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sel, ok := s.pending[ownerID]; ok {
		switch {
		case !draining:
			sel.held = append(sel.held, retry)
			return nil, false
		case sel.selections > 0:
			sel.held = append([]func(){retry}, sel.held...)
			return nil, false
		}
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
	l.settleLeftOwnerPart(ownerID, online, offline, false)
}

// settleLeftOwnerPart is settleWithLeftOwner, with draining set when the
// selection gate runs it again as a held settle.
func (l *GameClientLink) settleLeftOwnerPart(ownerID int32, online func(*livePlayer), offline func(), draining bool) {
	retry := func() { l.settleLeftOwnerPart(ownerID, online, offline, true) }
	owner, ok := l.selections.settle(ownerID, l.livePlayerByID, offline, retry, draining)
	if !ok {
		return
	}
	// A session found leaving settles offline instead: on its queue, as a
	// settle arriving now; on a refused post, still in this settle's place.
	leaving := func(draining bool) { l.selections.settle(ownerID, nil, offline, retry, draining) }
	posted := postLive(owner, func() {
		if owner.detached() {
			leaving(false)
			return
		}
		online(owner)
	})
	if !posted {
		leaving(draining)
	}
}
