package manager

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func pickPosition(positions []spawn.Position) spawn.Position {
	if len(positions) == 1 {
		return positions[0]
	}

	chance := rnd.Get(100)
	for _, pos := range positions {
		chance -= pos.Chance
		if chance < 0 {
			pos.Heading = rnd.Get(65536)
			return pos
		}
	}
	last := positions[len(positions)-1]
	last.Heading = rnd.Get(65536)
	return last
}

func (n *Npcs) pickSpawnPosition(maker *spawn.Maker, entry spawn.Entry) (spawn.Position, bool) {
	if len(entry.Positions) > 0 {
		return pickPosition(entry.Positions), true
	}
	return randomTerritoryPosition(maker, n.geo)
}

const territorySpawnAttempts = 10
const territoryPointAttempts = 64

func randomTerritoryPosition(maker *spawn.Maker, geo move.Geo) (spawn.Position, bool) {
	if maker == nil || len(maker.Territories) == 0 {
		return spawn.Position{}, false
	}

	var last spawn.Position
	haveLast := false
	for i := 0; i < territorySpawnAttempts; i++ {
		territory := maker.Territories[rnd.Get(len(maker.Territories))]
		x, y, ok := randomPointInTerritory(territory)
		if !ok {
			continue
		}

		z := int(geo.Height(x, y, averageZ(territory)))
		pos := spawn.Position{
			Location: location.Location{X: x, Y: y, Z: z},
			Heading:  rnd.Get(65536),
		}
		last, haveLast = pos, true

		if z < territory.MinZ || z > territory.MaxZ || insideAnyTerritory(maker.BannedTerritories, pos.Location) {
			continue
		}
		return pos, true
	}
	return last, haveLast
}

func randomPointInTerritory(territory *spawn.Territory) (int, int, bool) {
	if territory == nil || len(territory.Nodes) == 0 {
		return 0, 0, false
	}

	minX, maxX := territory.Nodes[0].X, territory.Nodes[0].X
	minY, maxY := territory.Nodes[0].Y, territory.Nodes[0].Y
	for _, node := range territory.Nodes[1:] {
		minX = min(minX, node.X)
		maxX = max(maxX, node.X)
		minY = min(minY, node.Y)
		maxY = max(maxY, node.Y)
	}

	for i := 0; i < territoryPointAttempts; i++ {
		x := rnd.GetRange(minX, maxX)
		y := rnd.GetRange(minY, maxY)
		if territoryContains2D(territory, x, y) {
			return x, y, true
		}
	}

	x, y := territoryCentroid(territory)
	if territoryContains2D(territory, x, y) {
		return x, y, true
	}
	return 0, 0, false
}

func insideAnyTerritory(territories []*spawn.Territory, loc location.Location) bool {
	for _, territory := range territories {
		if territoryContainsLocation(territory, loc) {
			return true
		}
	}
	return false
}

func territoryContainsLocation(territory *spawn.Territory, loc location.Location) bool {
	return territory != nil &&
		loc.Z >= territory.MinZ &&
		loc.Z <= territory.MaxZ &&
		territoryContains2D(territory, loc.X, loc.Y)
}

func territoryContains2D(territory *spawn.Territory, x, y int) bool {
	nodes := territory.Nodes
	inside := false
	j := len(nodes) - 1
	for i := range nodes {
		a, b := nodes[i], nodes[j]
		if pointOnSegment(x, y, a, b) {
			return true
		}
		if (a.Y > y) != (b.Y > y) {
			crossX := float64(b.X-a.X)*float64(y-a.Y)/float64(b.Y-a.Y) + float64(a.X)
			if float64(x) < crossX {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func pointOnSegment(x, y int, a, b spawn.Node) bool {
	cross := (x-a.X)*(b.Y-a.Y) - (y-a.Y)*(b.X-a.X)
	if cross != 0 {
		return false
	}
	return x >= min(a.X, b.X) && x <= max(a.X, b.X) &&
		y >= min(a.Y, b.Y) && y <= max(a.Y, b.Y)
}

func territoryCentroid(territory *spawn.Territory) (int, int) {
	var x, y int
	for _, node := range territory.Nodes {
		x += node.X
		y += node.Y
	}
	return x / len(territory.Nodes), y / len(territory.Nodes)
}

func averageZ(territory *spawn.Territory) int {
	return (territory.MinZ + territory.MaxZ) / 2
}

// locatedRef and creatureActorRef are forward references that break the
// construction cycle between a live NPC and the movement/attack
// controllers it owns: the controllers need the NPC's position/combat
// surface, but the NPC's own constructor needs the controllers already
// built. Each embeds its target interface unset, is handed to the
// controller constructors, and is pointed at the real NPC immediately
// after — before anything can call through it.
