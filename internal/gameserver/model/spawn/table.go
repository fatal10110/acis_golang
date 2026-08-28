package spawn

import (
	"errors"
	"sort"
	"strings"
)

// Table is the full in-memory result of loading spawnlist XML files.
type Table struct {
	territories    map[string]*Territory
	territoryCount int
	makers         map[string]*Maker
	order          []*Maker
}

// NewTable builds a Table from already-validated territories and makers.
func NewTable(territories []*Territory, makers []*Maker) (*Table, error) {
	if len(territories) == 0 {
		return nil, errors.New("spawn: table has no territories")
	}
	if len(makers) == 0 {
		return nil, errors.New("spawn: table has no makers")
	}

	// Java's SpawnManager.getTerritory/getNpcMaker resolve case-insensitively
	// (equalsIgnoreCase), returning the first declared match. Index by the
	// lowercased name so lookups match that contract; keep insertion order
	// (source declaration order) as the deterministic tie-breaker.
	territoryMap := make(map[string]*Territory, len(territories))
	for _, territory := range territories {
		key := strings.ToLower(territory.Name)
		if _, exists := territoryMap[key]; !exists {
			territoryMap[key] = territory
		}
	}

	makerMap := make(map[string]*Maker, len(makers))
	for _, maker := range makers {
		key := strings.ToLower(maker.Name)
		if _, exists := makerMap[key]; exists {
			return nil, errors.New("spawn: duplicate maker " + maker.Name)
		}
		makerMap[key] = maker
	}

	order := append([]*Maker(nil), makers...)
	sort.Slice(order, func(i, j int) bool { return order[i].Name < order[j].Name })

	return &Table{
		territories:    territoryMap,
		territoryCount: len(territories),
		makers:         makerMap,
		order:          order,
	}, nil
}

// Territory returns one territory by name, matched case-insensitively.
func (t *Table) Territory(name string) (*Territory, bool) {
	territory, ok := t.territories[strings.ToLower(name)]
	return territory, ok
}

// Maker returns one maker by name, matched case-insensitively.
func (t *Table) Maker(name string) (*Maker, bool) {
	maker, ok := t.makers[strings.ToLower(name)]
	return maker, ok
}

// TerritoryCount returns the number of territories loaded.
func (t *Table) TerritoryCount() int {
	return t.territoryCount
}

// MakerCount returns the number of makers loaded.
func (t *Table) MakerCount() int {
	return len(t.makers)
}

// SpawnCount returns the number of <npc> entries across every maker.
func (t *Table) SpawnCount() int {
	total := 0
	for _, maker := range t.order {
		total += len(maker.Entries)
	}
	return total
}

// Makers returns every maker ordered by name.
func (t *Table) Makers() []*Maker {
	return append([]*Maker(nil), t.order...)
}
