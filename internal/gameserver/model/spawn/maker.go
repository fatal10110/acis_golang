package spawn

import "math"

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

// NewMaker returns m as the spawnlist loads it: MaximumNPCs is scaled and
// rounded by multiplier, the configured spawn multiplier (see
// scaleBySpawnMultiplier). Empty Territories, BannedTerritories, Entries and
// AIParams read as nil.
//
// A territory name that resolves to nothing still builds the maker; a maker
// with no resolvable territory just never finds a spawn position
// (randomTerritoryPosition already treats a nil or empty Territories the same
// way), matching that tolerance instead of failing the whole spawnlist load.
func NewMaker(m Maker, multiplier float64) *Maker {
	m.Territories = append([]*Territory(nil), m.Territories...)
	m.BannedTerritories = append([]*Territory(nil), m.BannedTerritories...)
	m.AIParams = copyStringMap(m.AIParams)
	m.MaximumNPCs = scaleBySpawnMultiplier(m.MaximumNPCs, multiplier)
	m.Entries = append([]Entry(nil), m.Entries...)
	return &m
}

// scaleBySpawnMultiplier applies the spawn multiplier as n * multiplier
// rounded half up to an int. math.Round rounds half away from zero, which
// is the same rule for the non-negative n this is always called with.
func scaleBySpawnMultiplier(n int, multiplier float64) int {
	return int(math.Round(float64(n) * multiplier))
}
