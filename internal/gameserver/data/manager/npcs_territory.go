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

// randomTerritoryPosition matches Java's SpawnManager.findTerritory (merges
// a maker's ";"-delimited territory list into one Territory: minZ/maxZ are
// min-of-mins/max-of-maxes across the list, shapes are the union) plus
// Territory.getRandomLocation's area-weighted triangle draw
// (Territory.java:103-156, Polygon.java:15-37: a triangle is picked with
// probability proportional to its size within the merged shape list). Since
// each sub-territory's contribution to that merged weight is proportional
// to its own total area, picking a whole sub-territory with probability
// proportional to territoryArea, then a uniform point within it, is the
// same distribution. Z is resolved and range-checked against the merged
// bounds, not the picked sub-territory's own MinZ/MaxZ.
func randomTerritoryPosition(maker *spawn.Maker, geo move.Geo) (spawn.Position, bool) {
	if maker == nil || len(maker.Territories) == 0 {
		return spawn.Position{}, false
	}

	minZ, maxZ := mergedZRange(maker.Territories)
	avgZ := (minZ + maxZ) / 2

	var last spawn.Position
	haveLast := false
	for i := 0; i < territorySpawnAttempts; i++ {
		territory := weightedTerritoryPick(maker.Territories)
		x, y, ok := randomPointInTerritory(territory)
		if !ok {
			continue
		}

		z := int(geo.Height(x, y, avgZ))
		pos := spawn.Position{
			Location: location.Location{X: x, Y: y, Z: z},
			Heading:  rnd.Get(65536),
		}
		last, haveLast = pos, true

		if z < minZ || z > maxZ || insideAnyTerritory(maker.BannedTerritories, pos.Location) {
			continue
		}
		return pos, true
	}
	return last, haveLast
}

func mergedZRange(territories []*spawn.Territory) (minZ, maxZ int) {
	minZ, maxZ = territories[0].MinZ, territories[0].MaxZ
	for _, t := range territories[1:] {
		minZ = min(minZ, t.MinZ)
		maxZ = max(maxZ, t.MaxZ)
	}
	return minZ, maxZ
}

// weightedTerritoryPick draws one territory with probability proportional
// to its 2D polygon area, matching Java's area/size-weighted triangle
// selection over the merged shape list (see randomTerritoryPosition).
func weightedTerritoryPick(territories []*spawn.Territory) *spawn.Territory {
	if len(territories) == 1 {
		return territories[0]
	}

	total := 0.0
	areas := make([]float64, len(territories))
	for i, t := range territories {
		areas[i] = territoryArea(t)
		total += areas[i]
	}
	if total <= 0 {
		return territories[rnd.Get(len(territories))]
	}

	roll := rnd.GetFloat(total)
	for i, area := range areas {
		roll -= area
		if roll < 0 {
			return territories[i]
		}
	}
	return territories[len(territories)-1]
}

// territoryArea is the shoelace-formula area of the territory's polygon,
// computed from Nodes directly so it agrees with territoryContains2D's own
// point-in-polygon test regardless of whether the embedded geometry.Territory
// was populated.
func territoryArea(territory *spawn.Territory) float64 {
	nodes := territory.Nodes
	if len(nodes) < 3 {
		return 0
	}
	var sum float64
	j := len(nodes) - 1
	for i := range nodes {
		sum += float64(nodes[j].X)*float64(nodes[i].Y) - float64(nodes[i].X)*float64(nodes[j].Y)
		j = i
	}
	if sum < 0 {
		sum = -sum
	}
	return sum / 2
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

// locatedRef and creatureActorRef are forward references that break the
// construction cycle between a live NPC and the movement/attack
// controllers it owns: the controllers need the NPC's position/combat
// surface, but the NPC's own constructor needs the controllers already
// built. Each embeds its target interface unset, is handed to the
// controller constructors, and is pointed at the real NPC immediately
// after — before anything can call through it.
