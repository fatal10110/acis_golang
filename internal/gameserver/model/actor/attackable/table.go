package attackable

import "sync"

// row is a table entry as the shared core sees it: whom it is about and its
// hate.
type row[E any] interface {
	*E
	bind(attacker Combatant)
	attacker() Combatant
	hateRef() *float64
}

// table is what a HateTable and a ThreatTable share: the owner, and the
// entries per attacker object id that mu guards. An attacker's hit adds to
// an entry from the attacker's queue while the owner's AI reads it.
type table[E any, P row[E]] struct {
	owner Combatant

	mu      sync.RWMutex
	entries map[int32]*E
}

func (t *table[E, P]) init(owner Combatant) {
	t.owner = owner
	t.entries = make(map[int32]*E)
}

// ignores reports whether hate from attacker is never recorded: a nil
// attacker, or a siege guard attacking a siege guard owner.
func (t *table[E, P]) ignores(attacker Combatant) bool {
	return attacker == nil || t.owner.SiegeGuard() && attacker.SiegeGuard()
}

// entryLocked returns attacker's entry, adding an empty one. The caller
// holds mu for writing.
func (t *table[E, P]) entryLocked(attacker Combatant) *E {
	e, ok := t.entries[attacker.ObjectID()]
	if !ok {
		e = new(E)
		P(e).bind(attacker)
		t.entries[attacker.ObjectID()] = e
	}
	return e
}

// mostHatedLocked returns the entry with the greatest hate, only among
// positive hate when positiveOnly, nil for none. The caller holds mu.
func (t *table[E, P]) mostHatedLocked(positiveOnly bool) *E {
	var best *E
	for _, e := range t.entries {
		h := *P(e).hateRef()
		if positiveOnly && h <= 0 {
			continue
		}
		if best == nil || h > *P(best).hateRef() {
			best = e
		}
	}
	return best
}

// mostHated returns a copy of mostHatedLocked's entry; ok is false when
// there is none or the owner is alike dead.
func (t *table[E, P]) mostHated(positiveOnly bool) (entry E, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.entries) == 0 || t.owner.AlikeDead() {
		return entry, false
	}
	best := t.mostHatedLocked(positiveOnly)
	if best == nil {
		return entry, false
	}
	return *best, true
}

func (t *table[E, P]) hate(target Combatant) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	e, ok := t.entries[target.ObjectID()]
	if !ok {
		return 0
	}
	return *P(e).hateRef()
}

func (t *table[E, P]) get(target Combatant) (entry E, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	e, ok := t.entries[target.ObjectID()]
	if !ok {
		return entry, false
	}
	return *e, true
}

func (t *table[E, P]) remove(target Combatant) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.entries, target.ObjectID())
}

// removeIf drops every entry whose attacker drop reports and returns those
// attackers. drop runs under mu.
func (t *table[E, P]) removeIf(drop func(Combatant) bool) []Combatant {
	t.mu.Lock()
	defer t.mu.Unlock()

	var removed []Combatant
	for id, e := range t.entries {
		if a := P(e).attacker(); drop(a) {
			delete(t.entries, id)
			removed = append(removed, a)
		}
	}
	return removed
}

// reduceAll subtracts amount from every entry's hate, with no lower bound.
func (t *table[E, P]) reduceAll(amount float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.entries {
		*P(e).hateRef() -= amount
	}
}

// zeroAll zeroes every entry's hate without dropping any entries.
func (t *table[E, P]) zeroAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.entries {
		*P(e).hateRef() = 0
	}
}

func (t *table[E, P]) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.entries)
}

func (t *table[E, P]) len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.entries)
}

// snapshot returns a copy of every entry, in no particular order.
func (t *table[E, P]) snapshot() []E {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]E, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, *e)
	}
	return out
}
