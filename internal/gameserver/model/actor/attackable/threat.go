package attackable

import (
	"sync"
	"time"
)

// maxThreatValue caps both the damage and the hate accumulated per attacker
// in a ThreatTable entry.
const maxThreatValue = 999999999

// Threat is one attacker's accumulated standing against a ThreatTable's
// owner: the damage it has dealt and the hate weight driving target
// selection, both capped at maxThreatValue, plus when it last dealt damage.
type Threat struct {
	Attacker  Combatant
	Damage    float64
	Hate      float64
	Timestamp time.Time
}

// ThreatTable accumulates per-attacker damage and hate for one NPC and
// selects the most hated attacker as its melee target. A ThreatTable never
// builds threat between two siege guards, and reports no most-hated
// attacker while its owner is alike dead.
//
// Aging out stale entries (dropping attackers that haven't dealt damage
// recently, or that left visibility) is a target-selection policy driven by
// the NPC's AI loop, not a concern of this table; ReduceAllHate and the
// Timestamp field give that loop what it needs to do so.
//
// mu guards entries.
type ThreatTable struct {
	owner Combatant

	mu      sync.RWMutex
	entries map[int32]*Threat
}

// NewThreatTable returns an empty ThreatTable for owner.
func NewThreatTable(owner Combatant) *ThreatTable {
	return &ThreatTable{owner: owner, entries: make(map[int32]*Threat)}
}

// AddDamage records damage dealt and hate raised by attacker. Both are
// added to any existing entry and capped at maxThreatValue; hate has no
// lower bound, so a negative delta (see ReduceAllHate) can still be
// applied through AddDamage with a zero damage component. A nil attacker,
// or an attacker that is a siege guard attacking another siege guard, is a
// no-op.
func (t *ThreatTable) AddDamage(attacker Combatant, damage, hate float64) {
	if attacker == nil {
		return
	}
	if t.owner.SiegeGuard() && attacker.SiegeGuard() {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	e, ok := t.entries[attacker.ObjectID()]
	if !ok {
		e = &Threat{Attacker: attacker}
		t.entries[attacker.ObjectID()] = e
	}
	e.Damage = min(e.Damage+damage, maxThreatValue)
	e.Hate = min(e.Hate+hate, maxThreatValue)
	e.Timestamp = time.Now()
}

// MostHated returns the attacker with the highest positive hate, or ok
// false if the table is empty, the owner is alike dead, or no attacker has
// positive hate.
func (t *ThreatTable) MostHated() (threat Threat, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.entries) == 0 || t.owner.AlikeDead() {
		return Threat{}, false
	}

	var best *Threat
	for _, e := range t.entries {
		if e.Hate <= 0 {
			continue
		}
		if best == nil || e.Hate > best.Hate {
			best = e
		}
	}
	if best == nil {
		return Threat{}, false
	}
	return *best, true
}

// Hate returns the owner's hate against target, or 0 if target is not in
// the table.
func (t *ThreatTable) Hate(target Combatant) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	e, ok := t.entries[target.ObjectID()]
	if !ok {
		return 0
	}
	return e.Hate
}

// Get returns target's full entry, or ok false if target is not in the
// table.
func (t *ThreatTable) Get(target Combatant) (threat Threat, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	e, ok := t.entries[target.ObjectID()]
	if !ok {
		return Threat{}, false
	}
	return *e, true
}

// StopHate zeroes target's hate without dropping its entry, so its damage
// and identity are preserved for reward calculation. It is a no-op if
// target is not in the table.
func (t *ThreatTable) StopHate(target Combatant) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if e, ok := t.entries[target.ObjectID()]; ok {
		e.Hate = 0
	}
}

// ReduceAllHate subtracts amount from every entry's hate. Unlike AddDamage,
// this has no lower bound: hate can go negative, which simply keeps that
// attacker out of MostHated until it accumulates positive hate again.
func (t *ThreatTable) ReduceAllHate(amount float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.entries {
		e.Hate -= amount
	}
}

// ZeroHate zeroes every entry's hate without dropping any entries.
func (t *ThreatTable) ZeroHate() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.entries {
		e.Hate = 0
	}
}

// Remove drops target's entry entirely.
func (t *ThreatTable) Remove(target Combatant) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.entries, target.ObjectID())
}

// Clear drops every entry.
func (t *ThreatTable) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.entries)
}

// IsEmpty reports whether the table has no entries.
func (t *ThreatTable) IsEmpty() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.entries) == 0
}

// Snapshot returns a copy of every entry, in no particular order.
func (t *ThreatTable) Snapshot() []Threat {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]Threat, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, *e)
	}
	return out
}
