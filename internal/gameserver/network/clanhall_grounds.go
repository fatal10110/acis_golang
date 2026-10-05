package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// WorldDoors are the doors spawned in the world: their state changes, and
// their lookup by name.
type WorldDoors interface {
	door.StateOwner
	// DoorByName returns the door named name, ignoring case.
	DoorByName(name string) (*door.Object, bool)
}

// clanHallBanishOffset is the random offset of a player thrown out of a
// clan hall's grounds (ClanHallZone.banishForeigners).
const clanHallBanishOffset = 20

// clanHallGrounds closes the clan halls' doors and clears their grounds
// through l.
type clanHallGrounds struct{ l *GameClientLink }

// ClanHallGrounds returns the grounds of the clan halls in l's world.
func ClanHallGrounds(l *GameClientLink) clanhall.Grounds { return clanHallGrounds{l: l} }

// CloseDoors closes each door named in gates (Residence.closeDoors), as
// Door.closeMe does: a door that is unknown, broken or closed already
// stays as it is.
func (g clanHallGrounds) CloseDoors(gates []string) {
	if g.l.doors == nil {
		return
	}
	for _, name := range gates {
		if d, ok := g.l.doors.DoorByName(name); ok {
			g.l.doors.SetDoorOpen(d.DoorID(), false)
		}
	}
}

// BanishForeigners throws every player who is not a member of clan clanID
// out of hall hallID's grounds (ClanHall.banishForeigners). The hall's
// grounds are the first clan hall zone naming it, as ClanHallManager links
// one zone to each hall.
func (g clanHallGrounds) BanishForeigners(hallID, clanID int32) {
	if g.l.zones == nil {
		return
	}
	for _, z := range zone.OfKind[*zone.ClanHall](g.l.zones) {
		if int32(z.ResidenceID) == hallID {
			z.BanishForeigners(clanID)
			return
		}
	}
}

// banishFromClanHall teleports a, a player thrown out of hall hallID's
// grounds, to a random banish point of the hall, within
// clanHallBanishOffset. The teleport runs on the player's own queue, so
// after the zone and hall locks the banishment ran under are released. A
// hall without a banish point leaves the player where it is.
func (l *GameClientLink) banishFromClanHall(hallID int32, a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok {
		return
	}
	hall, ok := l.clanHallData.Get(int(hallID))
	if !ok {
		return
	}
	spawns := hall.Spawns[residence.SpawnBanish]
	if len(spawns) == 0 {
		l.log.Warn().Int32("hall_id", hallID).Msg("clan hall: no banish point to throw a player out to")
		return
	}
	dest := spawns[rnd.Get(len(spawns))]
	live := za.live
	postLive(live, func() { l.teleportLivePlayer(live, dest, clanHallBanishOffset) })
}
