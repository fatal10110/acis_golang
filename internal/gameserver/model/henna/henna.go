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
	symbolID, err := set.GetInt("symbolId")
	if err != nil {
		return Henna{}, fmt.Errorf("henna: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("henna %d: %w", symbolID, err) }

	dyeID, err := set.GetInt32("dyeId")
	if err != nil {
		return Henna{}, wrap(err)
	}
	classes, err := set.GetIntArray("classes")
	if err != nil {
		return Henna{}, wrap(err)
	}
	price, err := set.GetIntDefault("price", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	intStat, err := set.GetIntDefault("INT", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	strStat, err := set.GetIntDefault("STR", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	conStat, err := set.GetIntDefault("CON", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	menStat, err := set.GetIntDefault("MEN", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	dexStat, err := set.GetIntDefault("DEX", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}
	witStat, err := set.GetIntDefault("WIT", 0)
	if err != nil {
		return Henna{}, wrap(err)
	}

	return Henna{
		SymbolID: symbolID, DyeID: dyeID, DrawPrice: price,
		INT: intStat, STR: strStat, CON: conStat, MEN: menStat, DEX: dexStat, WIT: witStat,
		Classes: classes,
	}, nil
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
