package effect

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// StatSlots is a creature's live stat Calculators, one per Stat, each
// created on first use. The zero value holds none, and its slot array is
// allocated with the first slot, so a creature whose stat chains never
// change holds no array. mu guards slot creation; each Calculator then
// guards its own Mods, so a warm read takes only mu's read lock. An
// attacker's formulas read a creature's stats from the attacker's queue.
type StatSlots struct {
	mu    sync.RWMutex
	calcs *[stat.Count]*Calculator
}

// Lookup returns s's Calculator, nil while s has none.
func (ss *StatSlots) Lookup(s stat.Stat) *Calculator {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.calcs == nil {
		return nil
	}
	return ss.calcs[s]
}

// Get returns s's Calculator, creating it with newCalc on first use. newCalc
// runs under the slots' lock, so it may take only leaf locks.
func (ss *StatSlots) Get(s stat.Stat, newCalc func(stat.Stat) Calculator) *Calculator {
	if calc := ss.Lookup(s); calc != nil {
		return calc
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.calcs == nil {
		ss.calcs = new([stat.Count]*Calculator)
	}
	if calc := ss.calcs[s]; calc != nil {
		return calc
	}
	calc := newCalc(s)
	ss.calcs[s] = &calc
	return &calc
}

// Attach adds each Mod of fns to its Stat's Calculator, created with
// newCalc on first use. Each Mod is published under its own Calculator's
// lock, so the batch is not atomic against a concurrent read.
func (ss *StatSlots) Attach(fns []Mod, newCalc func(stat.Stat) Calculator) {
	for _, fn := range fns {
		ss.Get(fn.Stat, newCalc).AddMod(fn)
	}
}

// RemoveOwner drops every Mod of owner and returns, in stat order, the Stat
// of each Mod dropped: a Stat repeats once per Mod. Only the stats owner
// owns (ModOwner.OwnedStats) are scanned.
func (ss *StatSlots) RemoveOwner(owner ModOwner) []stat.Stat {
	calcs := ss.snapshot()
	var removed []stat.Stat
	for s := range owner.OwnedStats() {
		calc := calcs[s]
		if calc == nil {
			continue
		}
		for range calc.RemoveOwner(owner) {
			removed = append(removed, s)
		}
	}
	return removed
}

// Owns reports whether any slot holds a Mod of owner.
func (ss *StatSlots) Owns(owner ModOwner) bool {
	calcs := ss.snapshot()
	for _, calc := range calcs {
		if calc != nil && calc.Owns(owner) {
			return true
		}
	}
	return false
}

// snapshot copies the slots as they are now.
func (ss *StatSlots) snapshot() [stat.Count]*Calculator {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.calcs == nil {
		return [stat.Count]*Calculator{}
	}
	return *ss.calcs
}
