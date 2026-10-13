package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// WorldDoors are the doors spawned in the world: their state changes,
// their lookup by name, and their reload.
type WorldDoors interface {
	door.StateOwner
	// Door returns the door of id.
	Door(id int) (*door.Object, bool)
	// DoorByName returns the door named name, ignoring case.
	DoorByName(name string) (*door.Object, bool)
	// ReloadDoors replaces the doors in the world with doors built from
	// the given table.
	ReloadDoors(doors *door.Table) error
	// SetDoorSiege makes s the siege of the residence door id belongs to,
	// on whatever door stands under that id.
	SetDoorSiege(id int, s door.Siege)
	// RespawnDoor stands door id again as a siege's door respawn does.
	RespawnDoor(id int, restoreHP float64, weak bool)
}

// clanHallBanishOffset is the random offset of a player thrown out of a
// clan hall's grounds.
const clanHallBanishOffset = 20

// clanHallGrounds closes the clan halls' doors and clears their grounds
// through l.
type clanHallGrounds struct{ l *GameClientLink }

// ClanHallGrounds returns the grounds of the clan halls in l's world.
func ClanHallGrounds(l *GameClientLink) clanhall.Grounds { return clanHallGrounds{l: l} }

// CloseDoors closes each door named in gates as any door closing does: a
// door that is unknown, broken or closed already stays as it is.
func (g clanHallGrounds) CloseDoors(gates []string) { g.l.setHallDoors(gates, false) }

// setHallDoors opens or closes each door named in gates, a clan hall's or
// a castle's gates list: a door that is unknown, broken or in that state
// already stays as it is.
func (l *GameClientLink) setHallDoors(gates []string, open bool) {
	if l.doors == nil {
		return
	}
	for _, name := range gates {
		if d, ok := l.doors.DoorByName(name); ok {
			l.doors.SetDoorOpen(d.DoorID(), open)
		}
	}
}

// BanishForeigners throws every player who is not a member of clan clanID
// out of hall hallID's grounds. The hall's grounds are the first clan hall
// zone naming it: each hall is linked to one zone.
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
