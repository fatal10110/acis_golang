package attackable

import "sync"

// HateEntry is one attacker's accumulated hate in a HateTable.
type HateEntry struct {
	Attacker Combatant
	Hate     float64
}

// HateTable accumulates per-attacker hate for one NPC and selects the most
// hated attacker as its spell-cast target. Unlike ThreatTable, hate here is
// not capped and carries no separate damage figure: it exists purely to
// rank casters by how provoked the NPC is. A HateTable never builds hate
// between two siege guards, and reports no most-hated attacker while its
// owner is alike dead.
//
// mu guards entries.
type HateTable struct {
	owner Combatant

	mu      sync.RWMutex
	entries map[int32]*HateEntry
}

// NewHateTable returns an empty HateTable for owner.
func NewHateTable(owner Combatant) *HateTable {
	return &HateTable{owner: owner, entries: make(map[int32]*HateEntry)}
}

// Add raises attacker's hate by amount, uncapped. A nil attacker, or an
// attacker that is a siege guard attacking another siege guard, is a
// no-op.
func (h *HateTable) Add(attacker Combatant, amount float64) {
	if attacker == nil {
		return
	}
	if h.owner.SiegeGuard() && attacker.SiegeGuard() {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	e, ok := h.entries[attacker.ObjectID()]
	if !ok {
		e = &HateEntry{Attacker: attacker}
		h.entries[attacker.ObjectID()] = e
	}
	e.Hate += amount
}

// MostHated returns the attacker with the highest hate. Unlike
// ThreatTable.MostHated, hate is not required to be positive: the entry
// with the greatest value wins even if every entry is zero or negative.
// ok is false if the table is empty or the owner is alike dead.
func (h *HateTable) MostHated() (entry HateEntry, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.entries) == 0 || h.owner.AlikeDead() {
		return HateEntry{}, false
	}

	var best *HateEntry
	for _, e := range h.entries {
		if best == nil || e.Hate > best.Hate {
			best = e
		}
	}
	return *best, true
}

// Hate returns the owner's hate against target, or 0 if target is not in
// the table.
func (h *HateTable) Hate(target Combatant) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	e, ok := h.entries[target.ObjectID()]
	if !ok {
		return 0
	}
	return e.Hate
}

// StopHate drops target's entry entirely.
func (h *HateTable) StopHate(target Combatant) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.entries, target.ObjectID())
}

// ReduceAllHate subtracts amount from every entry's hate, uncapped in
// either direction. Entries are kept even once negative.
func (h *HateTable) ReduceAllHate(amount float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, e := range h.entries {
		e.Hate -= amount
	}
}

// ZeroHate zeroes every entry's hate without dropping any entries.
func (h *HateTable) ZeroHate() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, e := range h.entries {
		e.Hate = 0
	}
}

// Clear drops every entry.
func (h *HateTable) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	clear(h.entries)
}

// IsEmpty reports whether the table has no entries.
func (h *HateTable) IsEmpty() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.entries) == 0
}

// Snapshot returns a copy of every entry, in no particular order.
func (h *HateTable) Snapshot() []HateEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	out := make([]HateEntry, 0, len(h.entries))
	for _, e := range h.entries {
		out = append(out, *e)
	}
	return out
}
