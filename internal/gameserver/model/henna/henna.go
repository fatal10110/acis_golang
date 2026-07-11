// Package henna models static dye symbol data loaded at boot.
package henna

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

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

// New builds a Henna from one folded <henna> element.
func New(set *commons.StatSet) (Henna, error) {
	idf := commons.NewFields(set, "henna")
	symbolID := idf.Int("symbolId")
	if err := idf.Err(); err != nil {
		return Henna{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("henna %d", symbolID))
	henna := Henna{
		SymbolID:  symbolID,
		DyeID:     f.Int32("dyeId"),
		DrawPrice: f.IntDefault("price", 0),
		INT:       f.IntDefault("INT", 0),
		STR:       f.IntDefault("STR", 0),
		CON:       f.IntDefault("CON", 0),
		MEN:       f.IntDefault("MEN", 0),
		DEX:       f.IntDefault("DEX", 0),
		WIT:       f.IntDefault("WIT", 0),
		Classes:   f.IntArray("classes"),
	}
	if err := f.Err(); err != nil {
		return Henna{}, err
	}
	return henna, nil
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
}

// NewTable builds a henna lookup table.
func NewTable(hennas []Henna) *Table {
	t := &Table{bySymbolID: make(map[int]Henna, len(hennas))}
	for _, h := range hennas {
		t.bySymbolID[h.SymbolID] = h
	}
	return t
}

// Len returns the number of hennas keyed by symbol id.
func (t *Table) Len() int {
	return len(t.bySymbolID)
}

// Find returns the henna with symbolID.
func (t *Table) Find(symbolID int) (Henna, bool) {
	h, ok := t.bySymbolID[symbolID]
	return h, ok
}
