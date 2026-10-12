package skill

import (
	"sort"
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
// it. The zero value is not usable; construct with NewTable.
type Table struct {
	defs atomic.Pointer[definitions]
}

// definitions is one loaded set of skill definitions with its max-level
// index. It is never changed once built.
type definitions struct {
	byID map[ID]map[int]Definition
	max  map[ID]int
}

// NewTable returns a Table backed by defs. A later entry silently overwrites
// an earlier one at the same id and level.
func NewTable(defs []Definition) *Table {
	d := &definitions{byID: make(map[ID]map[int]Definition), max: make(map[ID]int)}
	for _, def := range defs {
		levels, ok := d.byID[def.ID]
		if !ok {
			levels = make(map[int]Definition)
			d.byID[def.ID] = levels
		}
		levels[def.Level] = def

		if def.Level < enchantLevelFloor && def.Level > d.max[def.ID] {
			d.max[def.ID] = def.Level
		}
	}
	t := &Table{}
	t.defs.Store(d)
	return t
}

// Replace swaps t's definitions and their max-level index for from's, at
// once for every holder of t. A definition already resolved (an NPC
// template's skills, a running effect) keeps what it was read with; every
// lookup afterwards reads the new ones.
func (t *Table) Replace(from *Table) {
	t.defs.Store(from.defs.Load())
}

// Get returns the definition for id at level, or false if none was loaded.
func (t *Table) Get(id ID, level int) (Definition, bool) {
	levels, ok := t.defs.Load().byID[id]
	if !ok {
		return Definition{}, false
	}
	d, ok := levels[level]
	return d, ok
}

// Definition returns the definition for ref, satisfying the cast/handler
// packages' Definitions interface for a non-player caster (e.g. a hostile
// NPC) that has no per-character skill state to resolve through instead.
func (t *Table) Definition(ref Ref) (Definition, bool) {
	return t.Get(ref.ID, ref.Level)
}

// MaxLevel returns the highest regular (non-enchant) level loaded for id, or
// 0 if no definition was loaded for it.
func (t *Table) MaxLevel(id ID) int {
	return t.defs.Load().max[id]
}

// Len returns the total number of (id, level) definitions in the table.
func (t *Table) Len() int {
	return t.defs.Load().len()
}

func (d *definitions) len() int {
	n := 0
	for _, levels := range d.byID {
		n += len(levels)
	}
	return n
}

// All returns every loaded definition ordered by skill id, then level.
func (t *Table) All() []Definition {
	if t == nil {
		return nil
	}
	d := t.defs.Load()
	defs := make([]Definition, 0, d.len())
	for _, levels := range d.byID {
		for _, def := range levels {
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].ID != defs[j].ID {
			return defs[i].ID < defs[j].ID
		}
		return defs[i].Level < defs[j].Level
	})
	return defs
}

// TopLevels returns, for every skill id with a regular (non-enchant) level
// loaded, the definition at its highest regular level, ordered by id.
func (t *Table) TopLevels() []Definition {
	if t == nil {
		return nil
	}
	d := t.defs.Load()
	defs := make([]Definition, 0, len(d.max))
	for id, level := range d.max {
		if level > 0 {
			defs = append(defs, d.byID[id][level])
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return defs
}
