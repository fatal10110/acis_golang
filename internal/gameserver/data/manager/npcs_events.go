package manager

import (
	"slices"
)

// DefaultSpawnEvents is the npcs.properties SpawnEvents default: the extra
// and unique monsters, the lottery ticket sellers and the Miss Queen weapon
// dealers.
func DefaultSpawnEvents() []string {
	return []string{"extra_mob", "18age", "start_weapon"}
}

// spawnEvents is the SpawnEvents list in configured order, each name once
// and no empty name: an empty name never matches a maker's event, and a
// repeated one would start its makers twice.
type spawnEvents []string

func newSpawnEvents(names []string) spawnEvents {
	var out spawnEvents
	for _, name := range names {
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func (e spawnEvents) has(name string) bool {
	return name != "" && slices.Contains(e, name)
}
