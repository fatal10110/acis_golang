package script

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// The operations a script runs on the spawn that placed an NPC: bring the
// NPC back now, draw a new point of the spawn, read its flee point, change
// its respawn delay, and move every player standing in its territory.

// DoRespawn brings n back now through its spawn: the respawn its spawn has
// pending for it is cancelled, a database-tracked spawn's saved respawn
// time is cleared, and n is placed again, as a new life the handle
// follows, when it has left the world. Its spawn places it as any respawn
// does, at a point the spawn gives, whatever position n was last given. A
// live NPC, or a dead one whose corpse has not decayed yet, only has its
// pending respawn cancelled. A master's private comes back only while that
// master lives; otherwise nothing is cancelled. An NPC whose spawn is gone
// is left as it is. A handle on nothing panics.
func (s *Spawner) DoRespawn(n *NPC) {
	s.population().DoRespawn(memberOf(n, "respawn"))
}

// DrawSpawnLocation returns a point n's spawn would place it at now, and
// the way it would face there. An npcmaker spawn draws it as a respawn
// does, so a spawn with no declared point gives a new random point of its
// territory on every call; a private created at a point gives that point,
// and any other private a new point around its master, facing the
// master's way. It is not where n spawned this life (NPC.SpawnLocation).
// It panics for an NPC no spawn placed, for a territory with no point to
// draw, and for a handle on nothing.
func (s *Spawner) DrawSpawnLocation(n *NPC) Loc {
	at, heading, ok := s.population().DrawSpawnPoint(memberOf(n, "draw a spawn location"))
	if !ok {
		panic(fmt.Sprintf("script: the spawn of npc %d has no location to give", n.ObjectID()))
	}
	return Loc{X: at.X, Y: at.Y, Z: at.Z, Heading: heading}
}

// SetRespawnDelay sets the respawn delay of the spawn of n, a master's
// private, to seconds, 0 when negative. With no respawn delay the private
// leaves its master's privates when it decays. A private whose spawn is
// gone sets nothing; any other NPC panics, as for ScheduleRespawn.
func (s *Spawner) SetRespawnDelay(n *NPC, seconds int32) {
	if err := s.population().SetRespawnDelay(memberOf(n, "set the respawn delay"), time.Duration(seconds)*time.Second); err != nil {
		panic(fmt.Sprintf("script: set the respawn delay of npc %d: %v", n.ObjectID(), err))
	}
}

// FleeLocation returns the point the NPC's spawn names for it to flee to:
// the flee_x, flee_y and flee_z values of its npcmaker spawn's AI
// parameters, its template's never consulted. ok is false when all three
// are 0 or unset, and always for a private or a standalone spawn, whose
// spawn sets none. A value that is not an int panics.
func (n *NPC) FleeLocation() (Loc, bool) {
	x, y, z := n.spawnInt("flee_x"), n.spawnInt("flee_y"), n.spawnInt("flee_z")
	if x == 0 && y == 0 && z == 0 {
		return Loc{}, false
	}
	return Loc{X: x, Y: y, Z: z}, true
}

// spawnInt returns the int AI parameter name the NPC's spawn sets, 0 when
// unset. A value that is not an int panics.
func (n *NPC) spawnInt(name string) int {
	var (
		v  string
		ok bool
	)
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		v, ok = o.SpawnParam(name)
	case *npc.Folk:
		v, ok = o.SpawnParam(name)
	default:
		panic("script: the spawn parameters of a handle on nothing")
	}
	if !ok {
		return 0
	}
	i, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("script: spawn parameter %s of npc %d: %q is not an int", name, n.ObjectID(), v))
	}
	return int(i)
}

// SetXYZ puts the NPC at at, facing at's way, with no teleport and no
// packet of its own: the players around see it there only when the move
// takes it into another region, which it then leaves and enters as any
// move does. A decayed NPC is left as it is: its spawn places its next
// life at a point of its own. A handle on nothing panics.
func (n *NPC) SetXYZ(at Place) {
	l := at.loc()
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		if !o.Decayed() {
			o.SetXYZ(l.X, l.Y, l.Z)
			o.SetHeading(l.Heading)
		}
	case *npc.Folk:
		if !o.Decayed() {
			o.SetXYZ(l.X, l.Y, l.Z)
			o.SetHeading(l.Heading)
		}
	default:
		panic("script: set the position of a handle on nothing")
	}
}

// TeleportPlayersInTerritory teleports every player in the world standing
// in the territory of the spawn that placed n (NPC.InSpawnTerritory) to
// at, up to offset away on each axis, as Player.TeleportTo does: each on
// its own queue, once the calling task is done. The territory of a private
// holds every player.
func (s *Script) TeleportPlayersInTerritory(n *NPC, at Place, offset int) {
	if s.env == nil || s.env.World == nil {
		return
	}
	for _, o := range s.env.World.Players() {
		self, ok := o.(attackable.Combatant)
		if !ok {
			continue
		}
		if p := PlayerOf(self); n.InSpawnTerritory(p) {
			p.TeleportTo(at, offset)
		}
	}
}
