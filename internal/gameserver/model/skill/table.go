package skill

import (
	"cmp"
	"slices"
	"sync/atomic"
)

// enchantLevelFloor is the first enchant-level number a Definition can carry
// (levels below it are regular, non-enchanted levels). It is used to keep
// enchant levels out of MaxLevel, which reports a skill's highest regular
// level.
const enchantLevelFloor = 99

// Table is an in-memory lookup of skill definitions keyed by id and level,
// built at boot and replaced in place by //reload skill. Every read works on
// one loaded set of definitions, so a reload never shows a reader half of
// it. The zero value reads as an empty table; construct with NewTable.
//
// Lookups hand out pointers into the loaded set, which is shared by every
// caster: a Definition reached through a Table is read-only.
type Table struct {
	defs atomic.Pointer[definitions]
}

// definitions is one loaded set of skill definitions with its id index. It
// is never changed once built, so a pointer into defs stays valid and
// unchanged after a Replace drops the set.
type definitions struct {
	// defs holds one entry per (id, level), ordered by id, then level.
	defs []Definition
	byID map[ID]levelSpan
}

// levelSpan locates one skill id's levels in definitions.defs.
type levelSpan struct {
	start, end int
	// max is the highest regular (non-enchant) level, or 0 when the id has
	// only enchant levels.
	max int
}

// noDefinitions is what a Table reads before anything was stored in it.
var noDefinitions = &definitions{}

// snapshot returns the definitions every read of one call works on.
func (t *Table) snapshot() *definitions {
	if d := t.defs.Load(); d != nil {
		return d
	}
	return noDefinitions
}

// NewTable returns a Table holding a copy of defs. A later entry silently
// overwrites an earlier one at the same id and level.
func NewTable(defs []Definition) *Table {
	// Sort positions rather than the definitions themselves; the stable sort
	// keeps duplicates in input order, so the last of each run is the entry
	// that overwrites the others.
	order := make([]int, len(defs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return compareDefinitions(&defs[a], &defs[b]) })
	kept := order[:0]
	for _, i := range order {
		if n := len(kept); n > 0 && compareDefinitions(&defs[kept[n-1]], &defs[i]) == 0 {
			kept[n-1] = i
			continue
		}
		kept = append(kept, i)
	}

	d := &definitions{defs: make([]Definition, len(kept)), byID: make(map[ID]levelSpan)}
	for pos, i := range kept {
		d.defs[pos] = defs[i]
		def := &d.defs[pos]
		span, ok := d.byID[def.ID]
		if !ok {
			span.start = pos
		}
		span.end = pos + 1
		if def.Level < enchantLevelFloor && def.Level > span.max {
			span.max = def.Level
		}
		d.byID[def.ID] = span
	}
	t := &Table{}
	t.defs.Store(d)
	return t
}

func compareDefinitions(a, b *Definition) int {
	if c := cmp.Compare(a.ID, b.ID); c != 0 {
		return c
	}
	return cmp.Compare(a.Level, b.Level)
}

// Replace swaps t's definitions and their index for from's, at once for
// every holder of t. A definition already resolved (an NPC template's
// skills, a running effect) keeps what it was read with; every lookup
// afterwards reads the new ones.
func (t *Table) Replace(from *Table) {
	t.defs.Store(from.snapshot())
}

// Get returns the read-only definition for id at level, or false if none
// was loaded.
func (t *Table) Get(id ID, level int) (*Definition, bool) {
	return t.snapshot().get(id, level)
}

func (d *definitions) get(id ID, level int) (*Definition, bool) {
	span, ok := d.byID[id]
	if !ok {
		return nil, false
	}
	levels := d.defs[span.start:span.end]
	i, ok := slices.BinarySearchFunc(levels, level, func(def Definition, level int) int {
		return cmp.Compare(def.Level, level)
	})
	if !ok {
		return nil, false
	}
	return &levels[i], true
}

// Definition returns the read-only definition for ref, satisfying the
// cast/handler packages' Definitions interface for a non-player caster
// (e.g. a hostile NPC) that has no per-character skill state to resolve
// through instead.
func (t *Table) Definition(ref Ref) (*Definition, bool) {
	return t.Get(ref.ID, ref.Level)
}

// MaxLevel returns the highest regular (non-enchant) level loaded for id, or
// 0 if no definition was loaded for it.
func (t *Table) MaxLevel(id ID) int {
	return t.snapshot().byID[id].max
}

// Len returns the total number of (id, level) definitions in the table.
func (t *Table) Len() int {
	return len(t.snapshot().defs)
}

// All returns a copy of every loaded definition ordered by skill id, then
// level.
func (t *Table) All() []Definition {
	if t == nil {
		return nil
	}
	return slices.Clone(t.snapshot().defs)
}

// TopLevels returns, for every skill id with a regular (non-enchant) level
// loaded, a copy of the definition at its highest regular level, ordered by
// id.
func (t *Table) TopLevels() []Definition {
	if t == nil {
		return nil
	}
	d := t.snapshot()
	var defs []Definition
	for i := 0; i < len(d.defs); {
		span := d.byID[d.defs[i].ID]
		if span.max > 0 {
			if top, ok := d.get(d.defs[i].ID, span.max); ok {
				defs = append(defs, *top)
			}
		}
		i = span.end
	}
	return defs
}
