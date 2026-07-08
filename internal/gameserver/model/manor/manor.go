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
	cropID, err := set.GetInt("id")
	if err != nil {
		return Seed{}, fmt.Errorf("manor: seed: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("manor: seed crop %d: %w", cropID, err) }
	seedID, err := set.GetInt("seedId")
	if err != nil {
		return Seed{}, wrap(err)
	}
	matureID, err := set.GetInt("matureId")
	if err != nil {
		return Seed{}, wrap(err)
	}
	level, err := set.GetInt("level")
	if err != nil {
		return Seed{}, wrap(err)
	}
	reward1, err := set.GetInt("reward1")
	if err != nil {
		return Seed{}, wrap(err)
	}
	reward2, err := set.GetInt("reward2")
	if err != nil {
		return Seed{}, wrap(err)
	}
	castleID, err := set.GetInt("castleId")
	if err != nil {
		return Seed{}, wrap(err)
	}
	seedsLimit, err := set.GetInt("seedsLimit")
	if err != nil {
		return Seed{}, wrap(err)
	}
	cropsLimit, err := set.GetInt("cropsLimit")
	if err != nil {
		return Seed{}, wrap(err)
	}
	return Seed{
		CropID:      cropID,
		SeedID:      seedID,
		MatureID:    matureID,
		Level:       level,
		Reward1:     reward1,
		Reward2:     reward2,
		CastleID:    castleID,
		Alternative: set.GetBoolDefault("isAlternative", false),
		SeedsLimit:  seedsLimit,
		CropsLimit:  cropsLimit,
	}, nil
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
