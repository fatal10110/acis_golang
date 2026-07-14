// Package cubic models a player's active cubics: which fixed skills each
// cubic type casts, and the capped, ordered set of cubics a player can have
// active at once.
package cubic

import "sync"

// ID identifies which cubic type is active. Each type casts a fixed set of
// skills and is either offensive (fires at the owner's current target) or
// supportive (the life cubic heals the party).
type ID int

// The nine cubic types, in their shipped display order.
const (
	Storm       ID = 1
	Vampiric    ID = 2
	Life        ID = 3
	Viper       ID = 4
	Poltergeist ID = 5
	Binding     ID = 6
	Aqua        ID = 7
	Spark       ID = 8
	Attract     ID = 9
)

// SkillIDs returns the fixed skill ids a cubic of this type casts, in the
// order it rolls between them when more than one applies. Each skill's own
// level is resolved by the skill engine from the granting effect, not by
// this package.
func SkillIDs(id ID) []int {
	switch id {
	case Storm:
		return []int{4049}
	case Vampiric:
		return []int{4050}
	case Life:
		return []int{4051}
	case Viper:
		return []int{4052}
	case Poltergeist:
		return []int{4053, 4054, 4055}
	case Binding:
		return []int{4164}
	case Aqua:
		return []int{4165}
	case Spark:
		return []int{4166}
	case Attract:
		return []int{5115, 5116}
	default:
		return nil
	}
}

type entry struct {
	id           ID
	givenByOther bool
}

// List is a player's set of active cubics, capped at a caller-supplied slot
// count and ordered by grant time so the oldest entry is evicted first when
// a new cubic is admitted past the cap. The zero value is an empty, usable
// list. Mutations are guarded by mu, since a cubic's own admit/expire
// timers and an owner's command handler both touch it.
type List struct {
	mu      sync.RWMutex
	entries []entry
}

func (l *List) indexOf(id ID) int {
	for i, e := range l.entries {
		if e.id == id {
			return i
		}
	}
	return -1
}

// Has reports whether a cubic of id is currently active.
func (l *List) Has(id ID) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.indexOf(id) >= 0
}

// Len returns how many cubics are currently active.
func (l *List) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// AddOrRefresh admits a cubic of id (recording whether a party member
// granted it) unless one is already active, in which case the caller
// should reset that cubic's own expiry timer instead and refreshed reports
// true. Admitting past maxSlots evicts the oldest active cubic first;
// evicted and didEvict report which one, if any.
func (l *List) AddOrRefresh(id ID, givenByOther bool, maxSlots int) (refreshed bool, evicted ID, didEvict bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.indexOf(id) >= 0 {
		return true, 0, false
	}

	if len(l.entries) > maxSlots && len(l.entries) > 0 {
		evicted = l.entries[0].id
		didEvict = true
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry{id: id, givenByOther: givenByOther})
	return false, evicted, didEvict
}

// Remove deactivates the cubic of id, if active.
func (l *List) Remove(id ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.indexOf(id); i >= 0 {
		l.entries = append(l.entries[:i], l.entries[i+1:]...)
	}
}

// StopAll deactivates every active cubic and returns their ids.
func (l *List) StopAll() []ID {
	l.mu.Lock()
	defer l.mu.Unlock()

	ids := make([]ID, len(l.entries))
	for i, e := range l.entries {
		ids[i] = e.id
	}
	l.entries = nil
	return ids
}

// StopGivenByOthers deactivates every active cubic that was granted by
// another player and returns their ids, leaving the owner's own cubics
// active.
func (l *List) StopGivenByOthers() []ID {
	l.mu.Lock()
	defer l.mu.Unlock()

	var stopped []ID
	kept := l.entries[:0]
	for _, e := range l.entries {
		if e.givenByOther {
			stopped = append(stopped, e.id)
		} else {
			kept = append(kept, e)
		}
	}
	l.entries = kept
	return stopped
}
