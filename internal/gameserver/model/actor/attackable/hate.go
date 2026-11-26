package attackable

const (
	defaultHateAmount          = 100
	openingTerritoryHateAmount = 300
)

// HateEntry is one attacker's accumulated hate in a HateTable.
type HateEntry struct {
	Attacker Combatant
	Hate     float64
}

func (e *HateEntry) bind(attacker Combatant) { e.Attacker = attacker }
func (e *HateEntry) attacker() Combatant     { return e.Attacker }
func (e *HateEntry) hateRef() *float64       { return &e.Hate }

// HateTable accumulates per-attacker hate for one NPC and selects the most
// hated attacker as its spell-cast target. Unlike ThreatTable, hate here is
// not capped and carries no separate damage figure: it exists purely to
// rank casters by how provoked the NPC is. A HateTable never builds hate
// between two siege guards, and reports no most-hated attacker while its
// owner is alike dead. It is safe for concurrent use.
//
// A nil table is an empty one: it holds no hate and every method but Add
// and AddDefault reads nothing from it and changes nothing.
type HateTable struct {
	t table[HateEntry, *HateEntry]
}

// NewHateTable returns an empty HateTable for owner.
func NewHateTable(owner Combatant) *HateTable {
	h := &HateTable{}
	h.t.init(owner)
	return h
}

// Add raises attacker's hate by amount, uncapped. A nil attacker, or an
// attacker that is a siege guard attacking another siege guard, is a
// no-op.
func (h *HateTable) Add(attacker Combatant, amount float64) {
	if h.t.ignores(attacker) {
		return
	}
	h.t.mu.Lock()
	defer h.t.mu.Unlock()

	h.t.entryLocked(attacker).Hate += amount
}

// AddDefault raises attacker's hate by the default amount used when an NPC
// notices a target. The first target noticed while the owner is in its
// territory gets a larger opening value; all later targets use the base
// value.
func (h *HateTable) AddDefault(attacker Combatant, inTerritory bool) {
	if h.t.ignores(attacker) {
		return
	}
	h.t.mu.Lock()
	defer h.t.mu.Unlock()

	amount := float64(defaultHateAmount)
	if inTerritory && len(h.t.entries) == 0 {
		amount = openingTerritoryHateAmount
	}
	h.t.entryLocked(attacker).Hate += amount
}

// MostHated returns the attacker with the highest hate. Unlike
// ThreatTable.MostHated, hate is not required to be positive: the entry
// with the greatest value wins even if every entry is zero or negative.
// ok is false if the table is empty or the owner is alike dead.
func (h *HateTable) MostHated() (entry HateEntry, ok bool) {
	if h == nil {
		return HateEntry{}, false
	}
	return h.t.mostHated(false)
}

// Hate returns the owner's hate against target, or 0 if target is not in
// the table.
func (h *HateTable) Hate(target Combatant) float64 {
	if h == nil {
		return 0
	}
	return h.t.hate(target)
}

// Remove drops target's entry entirely.
func (h *HateTable) Remove(target Combatant) {
	if h == nil {
		return
	}
	h.t.remove(target)
}

// RefreshDropDead drops dead-equivalent attackers and attackers the owner
// no longer sees. It returns the attackers whose queued target desires
// should be removed.
func (h *HateTable) RefreshDropDead(visible func(Combatant) bool) []Combatant {
	if h == nil {
		return nil
	}
	return h.t.removeIf(func(a Combatant) bool {
		return a.AlikeDead() || (visible != nil && !visible(a))
	})
}

// RemoveIf drops every entry whose attacker drop reports, and returns
// those attackers. drop runs under the table's lock.
func (h *HateTable) RemoveIf(drop func(Combatant) bool) []Combatant {
	if h == nil {
		return nil
	}
	return h.t.removeIf(drop)
}

// ZeroHate zeroes every entry's hate without dropping any entries.
func (h *HateTable) ZeroHate() {
	if h == nil {
		return
	}
	h.t.zeroAll()
}

// Clear drops every entry.
func (h *HateTable) Clear() {
	if h == nil {
		return
	}
	h.t.clear()
}

// IsEmpty reports whether the table has no entries.
func (h *HateTable) IsEmpty() bool { return h.Len() == 0 }

// Len returns the number of entries.
func (h *HateTable) Len() int {
	if h == nil {
		return 0
	}
	return h.t.len()
}

// Snapshot returns a copy of every entry, in no particular order.
func (h *HateTable) Snapshot() []HateEntry {
	if h == nil {
		return nil
	}
	return h.t.snapshot()
}
