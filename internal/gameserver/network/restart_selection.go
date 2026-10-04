package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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

// restartPointDestination resolves where a restart-point request of
// requestType sends the dead live (RequestRestartPoint.portPlayer). A
// jailed player always restarts in the jail and a festival participant
// where they fell, whatever type they asked for. Only this request forces
// the jail; every other town teleport (//sendhome, a starved flying mount,
// a boss-zone ejection) still sends a jailed player to the nearest town.
//
// Neither castle nor clan hall sieges run yet (#234, #244): with no active
// siege, the castle restart needs the clan to own a castle, the siege HQ
// restart finds no flag and resolves to town, and no attacker waits out a
// respawn delay (#3346). The clan hall restart does not restore experience
// from a rented restore-exp function yet (#3347).
func (l *GameClientLink) restartPointDestination(live *livePlayer, requestType int32) (location.Location, restartOutcome) {
	switch {
	case live.Jailed():
		requestType = restartToJail
	case live.FestivalParticipant():
		requestType = restartFixed
	}

	switch requestType {
	case restartToClanHall:
		hallID := live.ClanHallID()
		if hallID == 0 {
			return location.Location{}, restartRefused
		}
		if hall, ok := l.clanHallData.Get(int(hallID)); ok {
			if at, ok := randomResidenceSpawn(hall.Spawns); ok {
				return at, restartResolved
			}
		}
	case restartToCastle:
		castleID := live.ClanCastleID()
		if castleID == 0 {
			return location.Location{}, restartRefused
		}
		if c, ok := l.castleData.Get(int(castleID)); ok {
			if at, ok := randomResidenceSpawn(c.Spawns); ok {
				return at, restartResolved
			}
		}
	case restartToSiegeHQ:
		// A siege flag only stands during a siege; without one the
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

	at, ok := l.restartDestination(live)
	if !ok {
		return location.Location{}, restartNoPoint
	}
	return at, restartResolved
}

// randomResidenceSpawn picks one of a residence's owner spawns at random
// (Residence.getRndSpawn(SpawnType.OWNER)).
func randomResidenceSpawn(spawns map[residence.SpawnType][]location.Location) (location.Location, bool) {
	owners := spawns[residence.SpawnOwner]
	if len(owners) == 0 {
		return location.Location{}, false
	}
	return owners[rnd.Get(len(owners))], true
}

// dieOptions are the restart choices the death window of live offers: the
// clan hall and castle restarts while live's clan owns one, the fixed
// restart for an access level allowed it. The siege HQ restart and the
// castle restart of a siege defender wait on the siege engine (#3346).
func dieOptions(live *livePlayer) serverpackets.DieOptions {
	return serverpackets.DieOptions{
		ClanHall: live.ClanHallID() != 0,
		Castle:   live.ClanCastleID() != 0,
		FixedRes: live.accessLevel().AllowFixedRes,
	}
}
