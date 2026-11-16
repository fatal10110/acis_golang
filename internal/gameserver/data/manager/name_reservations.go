package manager

import (
	"strings"
	"sync"
)

// nameReservations holds the character names a creation or a rename is
// taking: from before it checks that no row holds the name until its row
// holds it or it gives up. The characters table has no unique key on the
// name, so this is what keeps two of them from both passing the check.
// Names are held lower-cased, as name lookups ignore case. The zero value
// holds no name.
type nameReservations struct {
	mu   sync.Mutex
	held map[string]struct{}
}

// reserve holds name, reporting false when another creation or rename
// already holds it. release, safe to call more than once, frees it.
func (n *nameReservations) reserve(name string) (release func(), ok bool) {
	key := strings.ToLower(name)
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, held := n.held[key]; held {
		return nil, false
	}
	if n.held == nil {
		n.held = make(map[string]struct{})
	}
	n.held[key] = struct{}{}
	return sync.OnceFunc(func() {
		n.mu.Lock()
		delete(n.held, key)
		n.mu.Unlock()
	}), true
}

// ReserveName holds name, matched case-insensitively, against every
// character creation and every other ReserveName until release is called,
// reporting false when one of them holds it already. A caller that renames
// a character reserves the new name before checking that no character
// holds it and releases it once the name is stored or the rename gives up,
// so no creation takes the name in between.
func (r *Roster) ReserveName(name string) (release func(), ok bool) {
	return r.names.reserve(name)
}
