package attackable

import (
	"slices"
	"time"
)

// maxThreatValue caps both the damage and the hate accumulated per attacker
// in a ThreatTable entry.
const maxThreatValue = 999999999

// randomizeHateBonus lifts a randomly chosen attacker this far above the
// most hated one's hate.
const randomizeHateBonus = 200

// Threat is one attacker's accumulated standing against a ThreatTable's
// owner: the damage it has dealt and the hate weight driving target
// selection, both capped at maxThreatValue, plus when it last dealt damage.
type Threat struct {
	Attacker  Combatant
	Damage    float64
	Hate      float64
	Timestamp time.Time
}

func (e *Threat) bind(attacker Combatant) { e.Attacker = attacker }
func (e *Threat) attacker() Combatant     { return e.Attacker }
func (e *Threat) hateRef() *float64       { return &e.Hate }

// ThreatTable accumulates per-attacker damage and hate for one NPC and
// selects the most hated attacker as its melee target. A ThreatTable never
// builds threat between two siege guards, and reports no most-hated
// attacker while its owner is alike dead. It is safe for concurrent use.
//
// A nil table is an empty one: it holds no threat and every method but
// AddDamage reads nothing from it and changes nothing.
type ThreatTable struct {
	t table[Threat, *Threat]
	// now stamps Threat.Timestamp.
	now func() time.Time
}

// NewThreatTable returns an empty ThreatTable for owner whose timestamps
// read now.
func NewThreatTable(owner Combatant, now func() time.Time) *ThreatTable {
	t := &ThreatTable{now: now}
	t.t.init(owner)
	return t
}

// AddDamage records damage dealt and hate raised by attacker. Both are
// added to any existing entry and capped at maxThreatValue; hate has no
// lower bound, so a negative delta (see ReduceAllHate) can still be
// applied through AddDamage with a zero damage component. A nil attacker,
// or an attacker that is a siege guard attacking another siege guard, is a
// no-op.
func (t *ThreatTable) AddDamage(attacker Combatant, damage, hate float64) {
	if t.t.ignores(attacker) {
		return
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()

	e := t.t.entryLocked(attacker)
	e.Damage = min(e.Damage+damage, maxThreatValue)
	e.Hate = min(e.Hate+hate, maxThreatValue)
	e.Timestamp = t.now()
}

// RandomizeAttack is attack randomization: among
// attackers other than the current most-hated with positive hate, it picks
// one passing valid and raises its hate to mostHated's hate plus 200,
// displacing mostHated as the new top target without altering mostHated's
// own hate. pick selects an index in [0, n) among the filtered candidates,
// letting the caller plug in its own randomness source. Reports whether a
// swap happened; a no-op when the table has fewer than two entries, the
// owner is alike dead, no attacker currently holds positive hate, or no
// candidate passes valid.
func (t *ThreatTable) RandomizeAttack(valid func(Combatant) bool, pick func(int) int) bool {
	if t == nil {
		return false
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()

	if len(t.t.entries) < 2 || t.t.owner.AlikeDead() {
		return false
	}

	mostHated := t.t.mostHatedLocked(true)
	if mostHated == nil {
		return false
	}

	var candidates []*Threat
	for _, e := range t.t.entries {
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
	// Map iteration order is randomized per Go's runtime; sort by attacker
	// id first so pick's index lands on
	// a reproducible candidate instead of a different one each call.
	slices.SortFunc(candidates, func(a, b *Threat) int {
		return int(a.Attacker.ObjectID() - b.Attacker.ObjectID())
	})

	chosen := candidates[pick(len(candidates))]
	chosen.Hate = min(chosen.Hate+(mostHated.Hate-chosen.Hate)+randomizeHateBonus, maxThreatValue)
	chosen.Timestamp = t.now()
	return true
}

// StopHate zeroes target's hate without dropping its entry, so its damage
// and identity are preserved for reward calculation. It is a no-op if
// target is not in the table.
func (t *ThreatTable) StopHate(target Combatant) {
	if t == nil {
		return
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()

	if e, ok := t.t.entries[target.ObjectID()]; ok {
		e.Hate = 0
	}
}

// MostHated returns the attacker with the highest positive hate, or ok
// false if the table is empty, the owner is alike dead, or no attacker has
// positive hate.
func (t *ThreatTable) MostHated() (threat Threat, ok bool) {
	if t == nil {
		return Threat{}, false
	}
	return t.t.mostHated(true)
}

// Hate returns the owner's hate against target, or 0 if target is not in
// the table.
func (t *ThreatTable) Hate(target Combatant) float64 {
	if t == nil {
		return 0
	}
	return t.t.hate(target)
}

// Get returns target's full entry, or ok false if target is not in the
// table.
func (t *ThreatTable) Get(target Combatant) (threat Threat, ok bool) {
	if t == nil {
		return Threat{}, false
	}
	return t.t.get(target)
}

// ReduceAllHate subtracts amount from every entry's hate. Unlike AddDamage,
// this has no lower bound: hate can go negative, which simply keeps that
// attacker out of MostHated until it accumulates positive hate again.
func (t *ThreatTable) ReduceAllHate(amount float64) {
	if t == nil {
		return
	}
	t.t.reduceAll(amount)
}

// RefreshZeroDead drops attackers the owner no longer sees and stops hate
// for dead-equivalent attackers while preserving their damage. It returns
// the attackers whose queued target desires should be removed.
func (t *ThreatTable) RefreshZeroDead(visible func(Combatant) bool) []Combatant {
	if t == nil {
		return nil
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()

	var changed []Combatant
	for id, e := range t.t.entries {
		if e.Attacker.AlikeDead() {
			e.Hate = 0
			changed = append(changed, e.Attacker)
			continue
		}
		if visible != nil && !visible(e.Attacker) {
			delete(t.t.entries, id)
			changed = append(changed, e.Attacker)
		}
	}
	return changed
}

// ZeroHate zeroes every entry's hate without dropping any entries.
func (t *ThreatTable) ZeroHate() {
	if t == nil {
		return
	}
	t.t.zeroAll()
}

// Remove drops target's entry entirely.
func (t *ThreatTable) Remove(target Combatant) {
	if t == nil {
		return
	}
	t.t.remove(target)
}

// Clear drops every entry.
func (t *ThreatTable) Clear() {
	if t == nil {
		return
	}
	t.t.clear()
}

// IsEmpty reports whether the table has no entries.
func (t *ThreatTable) IsEmpty() bool {
	if t == nil {
		return true
	}
	return t.t.len() == 0
}

// Snapshot returns a copy of every entry, in no particular order.
func (t *ThreatTable) Snapshot() []Threat {
	if t == nil {
		return nil
	}
	return t.t.snapshot()
}
