package admin

import "sync"

// GMList is the roster of online game masters. Each member is listed or
// hidden: a hidden GM is still a member, but only another GM sees it. P is
// whatever identifies an online player to the caller. The zero value is an
// empty list ready to use; mu guards members, which keeps registration
// order.
type GMList[P comparable] struct {
	mu      sync.Mutex
	members []GMEntry[P]
}

// GMEntry is one member of a GMList.
type GMEntry[P comparable] struct {
	Player P
	Hidden bool
}

// Add makes p a member, hidden or listed. A member already present keeps
// its place and takes the new hidden state.
func (l *GMList[P]) Add(p P, hidden bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.members {
		if l.members[i].Player == p {
			l.members[i].Hidden = hidden
			return
		}
	}
	l.members = append(l.members, GMEntry[P]{Player: p, Hidden: hidden})
}

// Remove drops p from the list; absent, it does nothing.
func (l *GMList[P]) Remove(p P) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.members {
		if l.members[i].Player == p {
			l.members = append(l.members[:i], l.members[i+1:]...)
			return
		}
	}
}

// Toggle flips member p between hidden and listed and returns its new
// hidden state. ok is false, and nothing changes, when p is not a member.
func (l *GMList[P]) Toggle(p P) (hidden, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.members {
		if l.members[i].Player == p {
			l.members[i].Hidden = !l.members[i].Hidden
			return l.members[i].Hidden, true
		}
	}
	return false, false
}

// Contains reports whether p is a member, hidden or not.
func (l *GMList[P]) Contains(p P) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.members {
		if m.Player == p {
			return true
		}
	}
	return false
}

// Entries returns a snapshot of the members, the hidden ones only when
// includeHidden is set.
func (l *GMList[P]) Entries(includeHidden bool) []GMEntry[P] {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]GMEntry[P], 0, len(l.members))
	for _, m := range l.members {
		if includeHidden || !m.Hidden {
			out = append(out, m)
		}
	}
	return out
}

// Online reports whether any member is online, counting hidden ones only
// when includeHidden is set.
func (l *GMList[P]) Online(includeHidden bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.members {
		if includeHidden || !m.Hidden {
			return true
		}
	}
	return false
}
