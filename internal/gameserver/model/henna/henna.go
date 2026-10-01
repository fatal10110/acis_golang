// Package henna models static dye symbol data loaded at boot.
package henna

import "sort"

// DrawAmount is the dye item count consumed when drawing a symbol.
const DrawAmount = 10

// RemoveAmount is the divisor used to compute the removal price.
const RemoveAmount = 5

// Henna is one dye symbol template.
type Henna struct {
	SymbolID  int
	DyeID     int32
	DrawPrice int
	INT       int
	STR       int
	CON       int
	MEN       int
	DEX       int
	WIT       int
	Classes   []int
}

// RemovePrice returns the adena cost to remove this symbol.
func (h Henna) RemovePrice() int {
	return h.DrawPrice / RemoveAmount
}

// UsableByClass reports whether classID is allowed to draw this symbol.
func (h Henna) UsableByClass(classID int) bool {
	for _, allowed := range h.Classes {
		if allowed == classID {
			return true
		}
	}
	return false
}

// Table stores hennas keyed by symbol id.
type Table struct {
	bySymbolID map[int]Henna
	ordered    []Henna
}

// NewTable builds a henna lookup table. A later row with an already seen
// symbol id replaces the earlier one.
func NewTable(hennas []Henna) *Table {
	t := &Table{bySymbolID: make(map[int]Henna, len(hennas))}
	for _, h := range hennas {
		t.bySymbolID[h.SymbolID] = h
	}
	t.ordered = make([]Henna, 0, len(t.bySymbolID))
	for _, h := range t.bySymbolID {
		t.ordered = append(t.ordered, h)
	}
	sort.Slice(t.ordered, func(i, j int) bool { return t.ordered[i].SymbolID < t.ordered[j].SymbolID })
	return t
}

// All returns every henna in ascending symbol id order, the order the
// symbol maker's draw list offers them in. The slice is shared; callers
// must not modify it.
func (t *Table) All() []Henna {
	if t == nil {
		return nil
	}
	return t.ordered
}

// Len returns the number of hennas keyed by symbol id.
func (t *Table) Len() int {
	return len(t.bySymbolID)
}

// Find returns the henna with symbolID.
func (t *Table) Find(symbolID int) (Henna, bool) {
	if t == nil {
		return Henna{}, false
	}
	h, ok := t.bySymbolID[symbolID]
	return h, ok
}
