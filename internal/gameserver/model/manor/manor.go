package manor

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Seed is one crop/seed row from manors.xml.
type Seed struct {
	CropID, SeedID, MatureID int
	Level                    int
	Reward1, Reward2         int
	CastleID                 int
	Alternative              bool
	SeedsLimit, CropsLimit   int
}

// NewSeed builds a Seed from set.
func NewSeed(set *commons.StatSet) (Seed, error) {
	idf := commons.NewFields(set, "manor: seed")
	cropID := idf.Int("id")
	if err := idf.Err(); err != nil {
		return Seed{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("manor: seed crop %d", cropID))
	seed := Seed{
		CropID:      cropID,
		SeedID:      f.Int("seedId"),
		MatureID:    f.Int("matureId"),
		Level:       f.Int("level"),
		Reward1:     f.Int("reward1"),
		Reward2:     f.Int("reward2"),
		CastleID:    f.Int("castleId"),
		Alternative: f.BoolDefault("isAlternative", false),
		SeedsLimit:  f.Int("seedsLimit"),
		CropsLimit:  f.Int("cropsLimit"),
	}
	if err := f.Err(); err != nil {
		return Seed{}, err
	}
	return seed, nil
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
}

// NewTable builds a manor seed table and rejects duplicate seed ids.
func NewTable(manors []Manor) (*Table, error) {
	t := &Table{
		Manors:    append([]Manor(nil), manors...),
		SeedsByID: make(map[int]Seed),
	}
	for _, m := range manors {
		for _, seed := range m.Seeds {
			if _, exists := t.SeedsByID[seed.SeedID]; exists {
				return nil, fmt.Errorf("manor: duplicate seed id %d", seed.SeedID)
			}
			t.SeedsByID[seed.SeedID] = seed
		}
	}
	return t, nil
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
