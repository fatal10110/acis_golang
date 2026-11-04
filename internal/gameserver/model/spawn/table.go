package spawn

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
)

// Table is the full in-memory result of loading spawnlist XML files.
type Table struct {
	territories    map[string]*Territory
	territoryCount int
	makers         map[string]*Maker
	// order is makers in list order: the order they were handed in,
	// which is the spawnlist's file and declaration order.
	order []*Maker
	// grouped are the merged territories of the makers naming several, by
	// maker: its territory and its banned territory, nil where it names
	// fewer than two.
	grouped map[*Maker][2]*Territory
}

// NewTable builds a Table from already-validated territories and makers,
// keeping makers in the order given.
func NewTable(territories []*Territory, makers []*Maker) (*Table, error) {
	if len(territories) == 0 {
		return nil, errors.New("spawn: table has no territories")
	}
	if len(makers) == 0 {
		return nil, errors.New("spawn: table has no makers")
	}

	// Territory and maker names resolve case-insensitively, returning the
	// first declared match. Index by the
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
	grouped := groupTerritories(order, territoryMap)

	return &Table{
		territories:    territoryMap,
		territoryCount: len(territories),
		makers:         makerMap,
		order:          order,
		grouped:        grouped,
	}, nil
}

// groupTerritories merges the territories of every maker naming several
// into one, named grouped_NNN by a counter that runs over the makers in
// list order, each maker's territory before its banned territory. Each
// merged territory spans the members' lowest to highest Z over all their
// triangles, and resolves by name in byName too.
func groupTerritories(makers []*Maker, byName map[string]*Territory) map[*Maker][2]*Territory {
	grouped := map[*Maker][2]*Territory{}
	next := 0
	merge := func(members []*Territory) *Territory {
		if len(members) < 2 {
			return nil
		}
		name := fmt.Sprintf("grouped_%03d", next)
		next++
		minZ, maxZ, ok := mergedZRange(members)
		var shapes []geometry.Shape
		for _, t := range members {
			if shape := t.geometryTerritory(); shape != nil {
				shapes = append(shapes, shape.Shapes...)
			}
		}
		shape, err := geometry.NewTerritory(minZ, maxZ, shapes...)
		if !ok || err != nil {
			return nil
		}
		t := &Territory{Name: name, MinZ: minZ, MaxZ: maxZ, Territory: shape}
		if _, taken := byName[name]; !taken {
			byName[name] = t
		}
		return t
	}
	for _, m := range makers {
		pair := [2]*Territory{merge(m.Territories), merge(m.BannedTerritories)}
		if pair[0] != nil || pair[1] != nil {
			grouped[m] = pair
		}
	}
	return grouped
}

// MakerTerritory returns the territory m spawns in as one: the only one it
// names, or the merged territory of the several it names. It is nil when m
// names none.
func (t *Table) MakerTerritory(m *Maker) *Territory {
	return t.territoryOf(m, m.Territories, 0)
}

// MakerBannedTerritory is MakerTerritory for m's banned territories.
func (t *Table) MakerBannedTerritory(m *Maker) *Territory {
	return t.territoryOf(m, m.BannedTerritories, 1)
}

func (t *Table) territoryOf(m *Maker, members []*Territory, i int) *Territory {
	switch len(members) {
	case 0:
		return nil
	case 1:
		return members[0]
	}
	return t.grouped[m][i]
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

// Makers returns every maker in list order: npcmakers start, and lookups
// that take the first match find them, in this order.
func (t *Table) Makers() []*Maker {
	return append([]*Maker(nil), t.order...)
}
