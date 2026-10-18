package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// The spawn lookups and spawn-time starts a script makes on the whole NPC
// population.

// Spawn is a spawn a script looked up by its NPC template: an npcmaker's
// spawn or a standalone one.
type Spawn struct {
	npcID int32
	l     spawn.Locator
}

// FindSpawn returns the first spawn of NPC template npcID: the first
// npcmaker spawn in list order, else a standalone spawn of it still kept,
// the one placed first; nil when there is none.
func (s *Spawner) FindSpawn(npcID int32) *Spawn {
	l := s.population().FindSpawn(npcID)
	if l == nil {
		return nil
	}
	return &Spawn{npcID: npcID, l: l}
}

// DrawSpawnLocation returns a point the spawn would place its NPC at now,
// and the way it would face there, drawn as Spawner.DrawSpawnLocation
// draws it: an npcmaker spawn with no declared point gives a new random
// point of its territory on every call. It panics for a territory with no
// point to draw.
func (sp *Spawn) DrawSpawnLocation() Loc {
	at, ok := sp.l.DrawSpawnLocation()
	if !ok {
		panic(fmt.Sprintf("script: the spawn of npc %d has no location to give", sp.npcID))
	}
	return Loc{X: at.Location.X, Y: at.Location.Y, Z: at.Location.Z, Heading: at.Heading}
}

// StartSpawnTime starts every npcmaker whose spawn time is name, ignoring
// case, and whose parameters match params, and returns how many NPCs those
// makers then have. A maker matches when each of its first three
// parameters equals, ignoring case, the one of params at the same place; a
// parameter params lacks matches none, so a maker with parameters never
// matches no params.
func (s *Spawner) StartSpawnTime(name string, params ...string) int {
	return s.population().StartSpawnTime(name, params...)
}

// StopSpawnTime deletes the NPCs of every npcmaker StartSpawnTime would
// start for name and params, and returns how many it deleted.
func (s *Spawner) StopSpawnTime(name string, params ...string) int {
	return s.population().StopSpawnTime(name, params...)
}
