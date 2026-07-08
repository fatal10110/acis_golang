package skill

import "sort"

// enchantLevelFloor is the first enchant-level number a Definition can carry
// (levels below it are regular, non-enchanted levels). It is used to keep
// enchant levels out of MaxLevel, which reports a skill's highest regular
// level.
const enchantLevelFloor = 99

// Table is an in-memory lookup of skill definitions keyed by id and level,
// built once at boot and read for the remainder of the process lifetime.
// The zero value is not usable; construct with NewTable.
type Table struct {
	byID map[ID]map[int]Definition
	max  map[ID]int
}

// NewTable returns a Table backed by defs. A later entry silently overwrites
// an earlier one at the same id and level.
func NewTable(defs []Definition) *Table {
	t := &Table{byID: make(map[ID]map[int]Definition), max: make(map[ID]int)}
	for _, d := range defs {
		levels, ok := t.byID[d.ID]
		if !ok {
			levels = make(map[int]Definition)
			t.byID[d.ID] = levels
		}
		levels[d.Level] = d

		if d.Level < enchantLevelFloor && d.Level > t.max[d.ID] {
			t.max[d.ID] = d.Level
		}
	}
	return t
}

// Get returns the definition for id at level, or false if none was loaded.
func (t *Table) Get(id ID, level int) (Definition, bool) {
	levels, ok := t.byID[id]
	if !ok {
		return Definition{}, false
	}
	d, ok := levels[level]
	return d, ok
}

// MaxLevel returns the highest regular (non-enchant) level loaded for id, or
// 0 if no definition was loaded for it.
func (t *Table) MaxLevel(id ID) int {
	return t.max[id]
}

// Len returns the total number of (id, level) definitions in the table.
func (t *Table) Len() int {
	n := 0
	for _, levels := range t.byID {
		n += len(levels)
	}
	return n
}

// All returns every loaded definition ordered by skill id, then level.
func (t *Table) All() []Definition {
	if t == nil {
		return nil
	}
	defs := make([]Definition, 0, t.Len())
	for _, levels := range t.byID {
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
