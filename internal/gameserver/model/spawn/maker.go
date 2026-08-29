package spawn

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Maker is one <npcmaker> group plus its resolved territory references.
type Maker struct {
	Name              string
	Territories       []*Territory
	BannedTerritories []*Territory
	AIType            string
	AIParams          map[string]string
	MaximumNPCs       int
	Event             string
	SpawnTime         string
	Entries           []Entry
}

// NewMaker builds a Maker from set plus already-resolved references and
// decoded child entries. multiplier is Config.SPAWN_MULTIPLIER
// (NpcMaker.java:89): maximumNpcs is scaled and Java-rounded by it.
func NewMaker(set *commons.StatSet, territories []*Territory, banned []*Territory, entries []Entry, aiParams map[string]string, multiplier float64) (*Maker, error) {
	idf := commons.NewFields(set, "spawn maker")
	name := idf.String("name")
	if err := idf.Err(); err != nil {
		return nil, err
	}
	f := commons.NewFields(set, fmt.Sprintf("spawn maker %q", name))
	maximum := f.Int("maximumNpcs")
	if err := f.Err(); err != nil {
		return nil, err
	}

	// SpawnManager.findTerritory resolves an unknown name to null and still
	// builds the NpcMaker; a maker with no resolvable territory just never
	// finds a spawn position (randomTerritoryPosition already treats a nil
	// or empty Territories the same way), matching that tolerance instead
	// of failing the whole spawnlist load.
	return &Maker{
		Name:              name,
		Territories:       append([]*Territory(nil), territories...),
		BannedTerritories: append([]*Territory(nil), banned...),
		AIType:            f.StringDefault("maker", ""),
		AIParams:          copyStringMap(aiParams),
		MaximumNPCs:       scaleBySpawnMultiplier(maximum, multiplier),
		Event:             f.StringDefault("event", ""),
		SpawnTime:         f.StringDefault("spawnTime", ""),
		Entries:           append([]Entry(nil), entries...),
	}, nil
}

// scaleBySpawnMultiplier applies Config.SPAWN_MULTIPLIER the way
// NpcMaker.java:89 and MultiSpawn.java:64 do: (int) Math.round(n *
// multiplier). Math.round rounds half up; math.Round rounds half away from
// zero, which is the same rule for the non-negative n this is always called
// with.
func scaleBySpawnMultiplier(n int, multiplier float64) int {
	return int(math.Round(float64(n) * multiplier))
}
