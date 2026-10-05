package manor

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// Seed is one crop/seed row from manors.xml. SeedReferencePrice and
// CropReferencePrice are the reference prices of its seed and crop items,
// set by Table.ApplyReferencePrices; SeedsLimit and CropsLimit are the
// unscaled manors.xml limits.
type Seed struct {
	CropID, SeedID, MatureID int
	Level                    int
	Reward1, Reward2         int
	CastleID                 int
	Alternative              bool
	SeedsLimit, CropsLimit   int

	SeedReferencePrice, CropReferencePrice int32
}

// Manor is one castle's seed list.
type Manor struct {
	ID    int
	Name  string
	Seeds []Seed
}

// Table stores manor seed rows keyed by seed id, plus the per-castle order.
type Table struct {
	Manors    []Manor
	SeedsByID map[int]Seed
	// order is every seed id in iteration order: see Seeds.
	order []int
}

// NewTable builds a manor seed table. A seed id repeated across manor rows
// keeps the last row, at the place of its first.
func NewTable(manors []Manor) *Table {
	t := &Table{
		Manors:    append([]Manor(nil), manors...),
		SeedsByID: make(map[int]Seed),
	}
	var ids []int
	for _, m := range manors {
		for _, seed := range m.Seeds {
			if _, dup := t.SeedsByID[seed.SeedID]; !dup {
				ids = append(ids, seed.SeedID)
			}
			t.SeedsByID[seed.SeedID] = seed
		}
	}
	t.order = hashOrder(ids)
	return t
}

// Area is one manor polygon assigned to a castle.
type Area struct {
	Name       string
	CastleID   int
	MinZ, MaxZ int
	Nodes      []location.Point
}

// AreaTable stores manor areas in file order.
type AreaTable []Area
