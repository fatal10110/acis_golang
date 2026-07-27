package attackable

import (
	"slices"
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

// RandomizeAttack ports Npc.java's AggroList.randomizeAttack(): among
// attackers other than the current most-hated with positive hate, it picks
// one passing valid and raises its hate to mostHated's hate plus 200,
// displacing mostHated as the new top target without altering mostHated's
// own hate. pick selects an index in [0, n) among the filtered candidates,
// letting the caller plug in its own randomness source. Reports whether a
// swap happened; a no-op when the table has fewer than two entries, the
// owner is alike dead, no attacker currently holds positive hate, or no
// candidate passes valid.
func (t *ThreatTable) RandomizeAttack(valid func(Combatant) bool, pick func(int) int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.entries) < 2 || t.owner.AlikeDead() {
		return false
	}

	var mostHated *Threat
	for _, e := range t.entries {
		if e.Hate <= 0 {
			continue
		}
		if mostHated == nil || e.Hate > mostHated.Hate {
			mostHated = e
		}
	}
	if mostHated == nil {
		return false
	}

	var candidates []*Threat
	for _, e := range t.entries {
		if e == mostHated || e.Hate <= 0 {
			continue
		}
		if valid(e.Attacker) {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	// Map iteration order is randomized per Go's runtime, unlike Java's
	// ConcurrentHashMap; sort by attacker id first so pick's index lands on
	// a reproducible candidate instead of a different one each call.
	slices.SortFunc(candidates, func(a, b *Threat) int {
		return int(a.Attacker.ObjectID() - b.Attacker.ObjectID())
	})

	chosen := candidates[pick(len(candidates))]
	chosen.Hate = min(chosen.Hate+(mostHated.Hate-chosen.Hate)+200, maxThreatValue)
	chosen.Timestamp = time.Now()
	return true
}

// ReconsiderTarget ports Npc.java's AggroList.reconsiderTarget(range): used
// when the owner can no longer act on its current target (e.g. an
// immobilize state) and must pick a replacement from its own hate list.
// Among attackers with positive hate other than the current most-hated,
// passing both inRange and valid, it picks the lowest-ObjectID candidate
// (Go map iteration order is unlike Java's ConcurrentHashMap, so entries are
// sorted for a reproducible pick instead of taking iteration's "first").
//
// If a most-hated attacker exists, its hate is zeroed and previousMostHated
// reports it so the caller can drop its queued attack desire; the chosen
// candidate's own hate is left unchanged — the reference reads
// mostHated.getHate() for the addDamageHate call only after already calling
// mostHated.stopHate() in the same statement sequence, so it always adds
// zero there. This is not a missed transfer to fix; it is the exact
// reference order, reproduced as coded. If no most-hated exists (empty hate
// list or an alike-dead owner, mirroring getMostHated's own dead check),
// the candidate instead gets a flat +2000 hate and previousMostHated is
// nil.
//
// Reports found false when the table has fewer than two entries or no
// candidate passes both filters.
func (t *ThreatTable) ReconsiderTarget(inRange func(Combatant) bool, valid func(Combatant) bool) (previousMostHated, chosen Combatant, found bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.entries) < 2 {
		return nil, nil, false
	}

	var mostHated *Threat
	if !t.owner.AlikeDead() {
		for _, e := range t.entries {
			if e.Hate <= 0 {
				continue
			}
			if mostHated == nil || e.Hate > mostHated.Hate {
				mostHated = e
			}
		}
	}

	var candidates []*Threat
	for _, e := range t.entries {
		if mostHated != nil && e == mostHated {
			continue
		}
		if e.Hate <= 0 {
			continue
		}
		if !inRange(e.Attacker) || !valid(e.Attacker) {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return nil, nil, false
	}
	slices.SortFunc(candidates, func(a, b *Threat) int {
		return int(a.Attacker.ObjectID() - b.Attacker.ObjectID())
	})
	picked := candidates[0]
	picked.Timestamp = time.Now()

	if mostHated == nil {
		picked.Hate = min(picked.Hate+2000, maxThreatValue)
		return nil, picked.Attacker, true
	}

	prevAttacker := mostHated.Attacker
	mostHated.Hate = 0
	return prevAttacker, picked.Attacker, true
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

// Refresh drops attackers the owner no longer sees and stops hate for
// dead-equivalent attackers while preserving their damage. It returns the
// attackers whose queued target desires should be removed.
func (t *ThreatTable) Refresh(visible func(Combatant) bool) []Combatant {
	t.mu.Lock()
	defer t.mu.Unlock()

	var changed []Combatant
	for id, e := range t.entries {
		if e.Attacker.AlikeDead() {
			e.Hate = 0
			changed = append(changed, e.Attacker)
			continue
		}
		if visible != nil && !visible(e.Attacker) {
			delete(t.entries, id)
			changed = append(changed, e.Attacker)
		}
	}
	return changed
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
