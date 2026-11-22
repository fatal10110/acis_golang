package zone

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Castle covers a castle's grounds.
type Castle struct {
	Zone
	// ResidenceID is the castle this zone belongs to.
	ResidenceID int
}

// NewCastle builds a castle zone; the loader sets ResidenceID from the
// zone's data settings before publishing it.
func NewCastle(id int, form Form) *Castle { return &Castle{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Castle) Core() *Zone { return &z.Zone }

func (z *Castle) affects(Actor) bool { return true }
func (z *Castle) enter(a Actor)      { a.ZoneFlags().Set(FlagCastle, true) }
func (z *Castle) exit(a Actor)       { a.ZoneFlags().Set(FlagCastle, false) }

// BanishForeigners throws every player not belonging to clanID out of the
// zone, through its effects.
func (z *Castle) BanishForeigners(clanID int32) {
	for _, a := range z.playersInside(func(a Actor) bool {
		p, ok := a.(Player)
		return ok && p.ClanID() != clanID
	}) {
		z.effects().CastleBanish(z, a)
	}
}

// ClanHall covers a clan hall's grounds.
type ClanHall struct {
	Zone
	// ResidenceID is the clan hall this zone belongs to.
	ResidenceID int
}

// NewClanHall builds a clan hall zone; the loader sets ResidenceID from the
// zone's data settings before publishing it.
func NewClanHall(id int, form Form) *ClanHall { return &ClanHall{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *ClanHall) Core() *Zone { return &z.Zone }

func (z *ClanHall) affects(Actor) bool { return true }

func (z *ClanHall) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagClanHall, true)
		z.effects().ShowHallInterior(z, a)
	}
}

func (z *ClanHall) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagClanHall, false)
	}
}

// BanishForeigners throws every player not belonging to clanID out of the
// zone, through its effects.
func (z *ClanHall) BanishForeigners(clanID int32) {
	for _, a := range z.playersInside(func(a Actor) bool {
		p, ok := a.(Player)
		return ok && p.ClanID() != clanID
	}) {
		z.effects().HallBanish(z, a)
	}
}

// CastleTeleport is the mass-gatekeeper room of a castle: friend summoning
// is blocked inside, and its occupants can be flushed to a random point of
// a configured exit box.
type CastleTeleport struct {
	Zone
	// CastleID is the castle this room belongs to.
	CastleID int
	// Exit box: occupants flushed from the room land on a random (x, y)
	// within these bounds at ExitZ.
	ExitMinX, ExitMaxX int
	ExitMinY, ExitMaxY int
	ExitZ              int
}

// NewCastleTeleport builds a mass-gatekeeper zone; the loader sets the
// castle link and exit box from the zone's data settings before publishing
// it.
func NewCastleTeleport(id int, form Form) *CastleTeleport {
	return &CastleTeleport{Zone: newZone(id, form)}
}

// Core exposes the shared zone state.
func (z *CastleTeleport) Core() *Zone { return &z.Zone }

func (z *CastleTeleport) affects(Actor) bool { return true }
func (z *CastleTeleport) enter(a Actor)      { a.ZoneFlags().Set(FlagNoSummonFriend, true) }
func (z *CastleTeleport) exit(a Actor)       { a.ZoneFlags().Set(FlagNoSummonFriend, false) }

// OustAll flushes every player inside to a random point of the exit box,
// through its effects.
func (z *CastleTeleport) OustAll() {
	for _, a := range z.playersInside(nil) {
		to := location.Location{
			X: randBetween(z.ExitMinX, z.ExitMaxX),
			Y: randBetween(z.ExitMinY, z.ExitMaxY),
			Z: z.ExitZ,
		}
		z.effects().TeleportRoomEject(z, a, to)
	}
}

// randBetween picks a uniform integer in [low, high], both ends inclusive.
func randBetween(low, high int) int {
	if high <= low {
		return low
	}
	return low + rand.IntN(high-low+1)
}
