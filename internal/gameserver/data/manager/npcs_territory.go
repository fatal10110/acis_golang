package manager

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func pickPosition(positions []spawn.Position) (spawn.Position, bool) {
	if len(positions) == 1 {
		return positions[0], true
	}

	chance := rnd.Get(100)
	for _, pos := range positions {
		chance -= pos.Chance
		if chance < 0 {
			pos.Heading = rnd.Get(65536)
			return pos, true
		}
	}
	return spawn.Position{}, false
}

func (n *Npcs) pickSpawnPosition(maker *spawn.Maker, entry spawn.Entry) (spawn.Position, bool) {
	if len(entry.Positions) > 0 {
		return pickPosition(entry.Positions)
	}
	return randomTerritoryPosition(maker, n.geo)
}

// randomTerritoryPosition places a territory-random spawn: a point from the
// maker's merged territory that avoids its merged banned territory, with a
// random heading.
func randomTerritoryPosition(maker *spawn.Maker, geo move.Geo) (spawn.Position, bool) {
	loc, ok := maker.RandomLocation(geo, true)
	if !ok {
		return spawn.Position{}, false
	}
	return spawn.Position{Location: loc, Heading: rnd.Get(65536)}, true
}

// locatedRef and creatureActorRef are forward references that break the
// construction cycle between a live NPC and the movement/attack
// controllers it owns: the controllers need the NPC's position/combat
// surface, but the NPC's own constructor needs the controllers already
// built. Each embeds its target interface unset, is handed to the
// controller constructors, and is pointed at the real NPC immediately
// after — before anything can call through it.
