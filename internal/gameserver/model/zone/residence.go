package zone

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Castle covers a castle's grounds.
type Castle struct {
	Zone
	// ResidenceID is the castle this zone belongs to.
	ResidenceID int
	// Banish teleports a player to the castle's banish spawn; nil until
	// the castle system wires it.
	Banish func(a Actor)
}

// NewCastle builds a castle zone from its data settings.
func NewCastle(id int, form Form, set *commons.StatSet) (*Castle, error) {
	castleID, err := set.GetIntDefault("castleId", 0)
	if err != nil {
		return nil, err
	}
	return &Castle{Zone: newZone(id, form), ResidenceID: castleID}, nil
}

// Core exposes the shared zone state.
func (z *Castle) Core() *Zone { return &z.Zone }

func (z *Castle) affects(Actor) bool { return true }
func (z *Castle) enter(a Actor)      { a.ZoneFlags().Set(FlagCastle, true) }
func (z *Castle) exit(a Actor)       { a.ZoneFlags().Set(FlagCastle, false) }

// BanishForeigners throws every player not belonging to clanID out of the
// zone, via the Banish hook.
func (z *Castle) BanishForeigners(clanID int32) {
	if z.Banish == nil {
		return
	}
	for _, a := range z.playersInside(func(a Actor) bool {
		p, ok := a.(Player)
		return ok && p.ClanID() != clanID
	}) {
		z.Banish(a)
	}
}

// ClanHall covers a clan hall's grounds.
type ClanHall struct {
	Zone
	// ResidenceID is the clan hall this zone belongs to.
	ResidenceID int
	// ShowInterior sends the hall's decoration state to an entering
	// player; nil until the clan hall system wires it.
	ShowInterior func(a Actor)
	// Banish teleports a player to the hall's banish spawn; nil until the
	// clan hall system wires it.
	Banish func(a Actor)
}

// NewClanHall builds a clan hall zone from its data settings.
func NewClanHall(id int, form Form, set *commons.StatSet) (*ClanHall, error) {
	hallID, err := set.GetIntDefault("clanHallId", 0)
	if err != nil {
		return nil, err
	}
	return &ClanHall{Zone: newZone(id, form), ResidenceID: hallID}, nil
}

// Core exposes the shared zone state.
func (z *ClanHall) Core() *Zone { return &z.Zone }

func (z *ClanHall) affects(Actor) bool { return true }

func (z *ClanHall) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagClanHall, true)
		if z.ShowInterior != nil {
			z.ShowInterior(a)
		}
	}
}

func (z *ClanHall) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagClanHall, false)
	}
}

// BanishForeigners throws every player not belonging to clanID out of the
// zone, via the Banish hook.
func (z *ClanHall) BanishForeigners(clanID int32) {
	if z.Banish == nil {
		return
	}
	for _, a := range z.playersInside(func(a Actor) bool {
		p, ok := a.(Player)
		return ok && p.ClanID() != clanID
	}) {
		z.Banish(a)
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
	// Eject teleports a player to the picked exit point; nil until the
	// teleport system wires it.
	Eject func(a Actor, to location.Location)
}

// NewCastleTeleport builds a mass-gatekeeper zone from its data settings.
func NewCastleTeleport(id int, form Form, set *commons.StatSet) (*CastleTeleport, error) {
	z := &CastleTeleport{Zone: newZone(id, form)}
	var err error
	if z.CastleID, err = set.GetIntDefault("castleId", 0); err != nil {
		return nil, err
	}
	if z.ExitMinX, err = set.GetIntDefault("spawnMinX", 0); err != nil {
		return nil, err
	}
	if z.ExitMaxX, err = set.GetIntDefault("spawnMaxX", 0); err != nil {
		return nil, err
	}
	if z.ExitMinY, err = set.GetIntDefault("spawnMinY", 0); err != nil {
		return nil, err
	}
	if z.ExitMaxY, err = set.GetIntDefault("spawnMaxY", 0); err != nil {
		return nil, err
	}
	if z.ExitZ, err = set.GetIntDefault("spawnZ", 0); err != nil {
		return nil, err
	}
	return z, nil
}

// Core exposes the shared zone state.
func (z *CastleTeleport) Core() *Zone { return &z.Zone }

func (z *CastleTeleport) affects(Actor) bool { return true }
func (z *CastleTeleport) enter(a Actor)      { a.ZoneFlags().Set(FlagNoSummonFriend, true) }
func (z *CastleTeleport) exit(a Actor)       { a.ZoneFlags().Set(FlagNoSummonFriend, false) }

// OustAll flushes every player inside to a random point of the exit box,
// via the Eject hook.
func (z *CastleTeleport) OustAll() {
	if z.Eject == nil {
		return
	}
	for _, a := range z.playersInside(nil) {
		to := location.Location{
			X: randBetween(z.ExitMinX, z.ExitMaxX),
			Y: randBetween(z.ExitMinY, z.ExitMaxY),
			Z: z.ExitZ,
		}
		z.Eject(a, to)
	}
}

// randBetween picks a uniform integer in [low, high], both ends inclusive.
func randBetween(low, high int) int {
	if high <= low {
		return low
	}
	return low + rand.IntN(high-low+1)
}
