package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/zonepulse"
)

var _ zone.Effects = (*zoneEffects)(nil)

// zoneEffects carries out what the zone rules of one zone index decide.
// The rule effects run under the occupant's zone membership, so each one
// that teleports, dismisses or moves a creature does it on that creature's
// queue; the occupant effects of NPCs and summons need the world, and do
// nothing without it.
type zoneEffects struct {
	l *GameClientLink
	// stadiums numbers the stadiums of the index as the tournament does.
	stadiums map[*zone.Olympiad]int
	pulses   *zonepulse.Pulses
}

// newZoneEffects builds the effects of the zones of ix.
func (l *GameClientLink) newZoneEffects(ix *zone.Index) *zoneEffects {
	fx := &zoneEffects{l: l, stadiums: map[*zone.Olympiad]int{}, pulses: l.zonePulsesOf(ix)}
	for i, z := range orderedStadiums(ix.All()) {
		fx.stadiums[z] = i
	}
	return fx
}

func (fx *zoneEffects) ArenaNotice(_ *zone.Arena, a zone.Actor, entering bool) {
	arenaCombatNotice(a, entering)
}

func (fx *zoneEffects) MotherTreeNotice(_ *zone.MotherTree, a zone.Actor, id int) {
	sendZoneMessage(a, id)
}

func (fx *zoneEffects) NoLandingDismount(_ *zone.NoLanding, a zone.Actor, entering bool) {
	fx.l.wyvernDismountTimer(a, entering)
}

func (fx *zoneEffects) ShowHallInterior(z *zone.ClanHall, a zone.Actor) {
	fx.l.showClanHallInterior(int32(z.ResidenceID), a)
}

func (fx *zoneEffects) SiegeNotice(_ *zone.Siege, a zone.Actor, entering bool) {
	fx.l.siegeCombatNotice(a, entering)
}

func (fx *zoneEffects) SiegeDismount(_ *zone.Siege, a zone.Actor, entering bool) {
	fx.l.siegeDismountTimer(a, entering)
}

func (fx *zoneEffects) SiegeFlagPvP(_ *zone.Siege, a zone.Actor) { fx.l.siegeFlagPvP(a) }

func (fx *zoneEffects) SiegeUnsummon(_ *zone.Siege, a zone.Actor) {
	if fx.l.world != nil {
		fx.l.unsummonSiegeOccupant(a)
	}
}

func (fx *zoneEffects) BossEject(z *zone.Boss, a zone.Actor) { fx.l.ejectBossPlayer(z, a) }

func (fx *zoneEffects) BossUnsummon(_ *zone.Boss, a zone.Actor) {
	if fx.l.world != nil {
		fx.l.unsummonZoneOccupant(a)
	}
}

func (fx *zoneEffects) BossRecallRaids(z *zone.Boss) {
	if fx.l.world == nil {
		return
	}
	for _, a := range z.Occupants() {
		fx.l.recallRaidOccupant(a)
	}
}

func (fx *zoneEffects) BossRecallNPC(_ *zone.Boss, a zone.Actor) {
	if fx.l.world != nil {
		fx.l.recallRaidOccupant(a)
	}
}

func (fx *zoneEffects) StadiumNotice(_ *zone.Olympiad, a zone.Actor, entering bool) {
	stadiumCombatNotice(a, entering)
}

func (fx *zoneEffects) StadiumMatchInfo(z *zone.Olympiad, a zone.Actor) {
	if id, ok := fx.stadiums[z]; ok {
		fx.l.sendStadiumMatchInfo(id, a)
	}
}

func (fx *zoneEffects) StadiumExpel(_ *zone.Olympiad, a zone.Actor) { fx.l.expelFromStadium(a) }

func (fx *zoneEffects) StartPulse(z zone.Kind) { fx.pulses.Start(z) }

func (fx *zoneEffects) CastleBanish(z *zone.Castle, a zone.Actor) {
	fx.l.banishFromCastle(z.ResidenceID, a)
}

func (fx *zoneEffects) HallBanish(z *zone.ClanHall, a zone.Actor) {
	fx.l.banishFromClanHall(int32(z.ResidenceID), a)
}

func (fx *zoneEffects) SiegeBanish(z *zone.Siege, a zone.Actor) { fx.l.banishFromSiege(z, a) }

func (fx *zoneEffects) TeleportRoomEject(_ *zone.CastleTeleport, a zone.Actor, to location.Location) {
	fx.l.ejectFromTeleportRoom(a, to)
}

func (fx *zoneEffects) StadiumFighting(z *zone.Olympiad) bool {
	id, ok := fx.stadiums[z]
	if !ok || fx.l.olympiad == nil {
		return false
	}
	s, ok := fx.l.olympiad.Stadium(id)
	return ok && s.BattleStarted()
}

func (fx *zoneEffects) CastleSiegeRunning(castleID int) bool {
	return fx.l.castleSiegeRunning(castleID)
}
