package network

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
)

// The spawn times of a castle's siege guards: its owner's makers, and the
// makers of a castle with no owner.
const (
	ownedSiegeGuards = "pc_siege_warfare_start"
	siegeGuards      = "siege_warfare_start"
)

// SpawnGuards spawns c's siege guards. With an owner, its owner's guard
// makers start, then each ticket on its grounds, newest first, hires a
// mercenary of the ticket's NPC where it lies, and the ticket leaves the
// world; a ticket that is not one of c's, or whose mercenary cannot be
// placed, stays on the ground. The castle's tickets are forgotten either
// way. With no owner, the ownerless castle's guard makers start.
func (n siegeNotices) SpawnGuards(c *castle.Castle) {
	l := n.l
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return
	}
	id := strconv.Itoa(c.ID)
	if c.OwnerID() <= 0 {
		npcs.StartSpawnTime(siegeGuards, id)
		return
	}
	npcs.StartSpawnTime(ownedSiegeGuards, id)
	for _, t := range c.TakeDroppedTickets() {
		ticket, ok := c.Ticket(t.ItemID)
		if !ok {
			continue
		}
		guard, err := npcs.SpawnMercenary(ticket.NPCID, t.Location, c.ID)
		if err != nil {
			l.log.Error().Err(err).Int("npc_id", ticket.NPCID).Int("castle", c.ID).Msg("siege: cannot spawn mercenary")
			continue
		}
		c.AddMercenary(guard)
		if ground, ok := l.resolveTarget(t.ObjectID).(*grounditem.Item); ok && ground.Claim() {
			l.despawnGround(ground)
		}
	}
}

// DespawnGuards deletes c's mercenaries, then the NPCs of both its guard
// makers.
func (n siegeNotices) DespawnGuards(c *castle.Castle) {
	npcs := n.l.npcSpawns.Load()
	if npcs == nil {
		return
	}
	for _, id := range c.TakeMercenaries() {
		npcs.DeleteFixed(id)
	}
	id := strconv.Itoa(c.ID)
	npcs.StopSpawnTime(ownedSiegeGuards, id)
	npcs.StopSpawnTime(siegeGuards, id)
}

// killMercenaries kills each mercenary c's siege hired, as if it struck
// itself down, and forgets them.
func (l *GameClientLink) killMercenaries(c *castle.Castle) {
	npcs := l.npcSpawns.Load()
	for _, id := range c.TakeMercenaries() {
		if npcs != nil {
			npcs.KillFixed(id)
		}
	}
}

// spawnTimes answers the world state the npcmakers with a spawn time
// follow through l.
type spawnTimes struct{ l *GameClientLink }

// SpawnTimes returns the world state, read through l, the npcmakers with a
// spawn time follow.
func SpawnTimes(l *GameClientLink) manager.SpawnTimes { return spawnTimes{l: l} }

// CastleSiege reports whether castle id's battlefield is active and
// whether a clan owns the castle.
func (s spawnTimes) CastleSiege(id int) (active, owned, ok bool) {
	c, ok := s.l.castles.Get(id)
	if !ok {
		return false, false, false
	}
	if sg, ok := s.l.sieges.Get(id); ok {
		if field := sg.Battlefield(); field != nil {
			active = field.Active()
		}
	}
	return active, c.OwnerID() > 0, true
}

// HallSiegeInProgress reports whether the siege of the siegable clan hall
// id is being fought.
//
// ponytail: no clan hall siege runs yet (#244), so none is ever in
// progress.
func (spawnTimes) HallSiegeInProgress(int) bool { return false }

// DoorOpen reports whether door id stands open.
func (s spawnTimes) DoorOpen(id int) bool {
	if s.l.doors == nil {
		return false
	}
	d, ok := s.l.doors.Door(id)
	return ok && d.Opened()
}
