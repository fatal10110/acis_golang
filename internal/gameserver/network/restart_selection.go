package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// The restart-point request types the death window and the server send.
const (
	restartToClanHall int32 = 1
	restartToCastle   int32 = 2
	restartToSiegeHQ  int32 = 3
	restartFixed      int32 = 4
	restartToJail     int32 = 27
)

// restartOutcome is how a restart-point request resolved.
type restartOutcome uint8

const (
	// restartResolved names a destination to revive the player at.
	restartResolved restartOutcome = iota
	// restartRefused is a selection the player may not use: nothing
	// happens and the player stays dead on the death screen.
	restartRefused
	// restartNoPoint is the town fallback finding no restart point at all.
	restartNoPoint
)

// effectiveRestartType is the restart type a request of requestType by
// live resolves as: a jailed player always
// restarts in the jail and a festival participant where they fell, whatever
// type they asked for. Only this request forces the jail; every other town
// teleport (//sendhome, a starved flying mount, a boss-zone ejection) still
// sends a jailed player to the nearest town.
func effectiveRestartType(live *livePlayer, requestType int32) int32 {
	switch {
	case live.Jailed():
		return restartToJail
	case live.FestivalParticipant():
		return restartFixed
	}
	return requestType
}

// restartPointDestination resolves where a restart-point request of
// requestType sends the dead live.
//
// The castle restart during a siege at live's position follows its clan's
// side: an owner or defender restarts at a castle, an attacker in town, and
// any other clan may not use it. Without a siege it needs the clan to own a
// castle.
//
// ponytail: the siege HQ restart to the clan's headquarters waits for
// #3346, so it always resolves to town; the clan hall siege branches wait
// for the hall sieges (#244).
//
// requestType is the type effectiveRestartType already enforced.
func (l *GameClientLink) restartPointDestination(live *livePlayer, requestType int32) (location.Location, restartOutcome) {
	switch requestType {
	case restartToClanHall:
		hallID := live.ClanHallID()
		if hallID == 0 {
			return location.Location{}, restartRefused
		}
		if at, ok := l.clanHallOwnerSpawn(hallID); ok {
			return at, restartResolved
		}
	case restartToCastle:
		if s, ok := l.siegeAt(live); ok {
			switch s.Side(live.ClanID()) {
			case siege.SideOwner, siege.SideDefender:
			case siege.SideAttacker:
				return l.townRestart(live)
			default:
				return location.Location{}, restartRefused
			}
		} else if live.ClanCastleID() == 0 {
			return location.Location{}, restartRefused
		}
		if castleID := l.restartCastleID(live); castleID != 0 {
			if at, ok := l.castleOwnerSpawn(castleID); ok {
				return at, restartResolved
			}
		}
	case restartToSiegeHQ:
		// The restart to the clan's headquarters waits for #3346; the
		// request falls back to town.
	case restartFixed:
		if !live.accessLevel().IsGM && !live.FestivalParticipant() {
			return location.Location{}, restartRefused
		}
		return live.CurrentLocation(), restartResolved
	case restartToJail:
		if !live.Jailed() {
			return location.Location{}, restartRefused
		}
		return jailLocation, restartResolved
	}
	return l.townRestart(live)
}

// townRestart is the town restart of live as a restart outcome.
func (l *GameClientLink) townRestart(live *livePlayer) (location.Location, restartOutcome) {
	at, ok := l.restartDestination(live)
	if !ok {
		return location.Location{}, restartNoPoint
	}
	return at, restartResolved
}

// siegeAt returns the siege in progress at live's position.
func (l *GameClientLink) siegeAt(live *livePlayer) (*siege.Siege, bool) {
	at := live.CurrentLocation()
	return l.sieges.ActiveAt(at.X, at.Y, at.Z)
}

// dawnHoldsStrife reports whether the Seven Signs are in seal validation
// with Dawn holding the Seal of Strife.
func (l *GameClientLink) dawnHoldsStrife() bool {
	return l.SealValidationPeriod() && l.sevenSigns.SealOwners()[sealStrifeIndex] == sevensigns.Dawn
}

// restartCastleID is the castle whose OWNER spawns a castle restart or
// recall of live lands at: its clan's own castle, else the castle besieged
// at live's position when its clan owns or defends it; 0 for none.
func (l *GameClientLink) restartCastleID(live *livePlayer) int32 {
	if id := live.ClanCastleID(); id != 0 {
		return id
	}
	clanID := live.ClanID()
	if clanID == 0 {
		return 0
	}
	if s, ok := l.siegeAt(live); ok && s.CheckSides(clanID, siege.SideDefender, siege.SideOwner) {
		return int32(s.Castle().ID)
	}
	return 0
}

// clanHallOwnerSpawn is a random OWNER spawn of clan hall hallID, false when
// the hall data does not know the hall or lists none.
func (l *GameClientLink) clanHallOwnerSpawn(hallID int32) (location.Location, bool) {
	hall, ok := l.clanHallData.Get(int(hallID))
	if !ok {
		return location.Location{}, false
	}
	return randomSpawn(hall.Spawns, residence.SpawnOwner)
}

// castleOwnerSpawn is a random OWNER spawn of castle castleID, false when
// the castle data does not know the castle or lists none.
func (l *GameClientLink) castleOwnerSpawn(castleID int32) (location.Location, bool) {
	c, ok := l.castleData.Get(int(castleID))
	if !ok {
		return location.Location{}, false
	}
	return randomSpawn(c.Spawns, residence.SpawnOwner)
}

// randomSpawn picks one of a residence's spawns of kind at random.
func randomSpawn(spawns map[residence.SpawnType][]location.Location, kind residence.SpawnType) (location.Location, bool) {
	list := spawns[kind]
	if len(list) == 0 {
		return location.Location{}, false
	}
	return list[rnd.Get(len(list))], true
}

// dieOptions are the restart choices the death window of live offers: the
// clan hall and castle restarts while live's clan owns one, the castle
// restart also while it owns or defends the siege in progress at live's
// position, and the fixed restart for an access level allowed it.
//
// ponytail: the siege HQ restart waits for #3346, and the clan hall siege
// branch for the hall sieges (#244).
func (l *GameClientLink) dieOptions(live *livePlayer) serverpackets.DieOptions {
	return serverpackets.DieOptions{
		ClanHall: live.ClanHallID() != 0,
		Castle:   l.restartCastleID(live) != 0,
		FixedRes: live.accessLevel().AllowFixedRes,
	}
}
