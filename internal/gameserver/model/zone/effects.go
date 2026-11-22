package zone

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// Effects carries out what the zone rules decide beyond an occupant's flag
// ledger, and answers what they ask of the rest of the server. Index.Wire
// gives one Effects to every zone of an index; a zone of an index never
// wired runs its rules on the ledger alone (NoEffects).
//
// The rule effects (every method but the queries and the four that throw
// players out on command) run inside the zone rule, with the occupant's
// zone membership locked, at the point of the rule that decides them: an
// effect must not change that membership synchronously, so one that
// teleports, dismisses or moves a creature runs it on that creature's
// queue.
type Effects interface {
	// ArenaNotice tells player a it entered or left arena z.
	ArenaNotice(z *Arena, a Actor, entering bool)
	// MotherTreeNotice sends player a crossing z the system message id.
	MotherTreeNotice(z *MotherTree, a Actor, id int)
	// NoLandingDismount starts (entering) or calls off the forced dismount
	// of a wyvern rider a over z.
	NoLandingDismount(z *NoLanding, a Actor, entering bool)
	// ShowHallInterior sends player a, entering z, the hall's decorations.
	ShowHallInterior(z *ClanHall, a Actor)

	// SiegeNotice tells player a it entered or left battlefield z under
	// siege.
	SiegeNotice(z *Siege, a Actor, entering bool)
	// SiegeDismount is NoLandingDismount for battlefield z under siege.
	SiegeDismount(z *Siege, a Actor, entering bool)
	// SiegeFlagPvP puts player a, leaving z under siege, on the pvp flag.
	SiegeFlagPvP(z *Siege, a Actor)
	// SiegeUnsummon dismisses a siege summon a that left z or outlasted
	// its siege.
	SiegeUnsummon(z *Siege, a Actor)

	// BossEject throws player a, entering z without leave, out of it.
	BossEject(z *Boss, a Actor)
	// BossUnsummon dismisses summon a, entering z while its owner has no
	// leave.
	BossUnsummon(z *Boss, a Actor)
	// BossRecallRaids sends every raid NPC inside z home: the last
	// playable left.
	BossRecallRaids(z *Boss)
	// BossRecallNPC sends a, an NPC leaving z, home when it is a raid NPC.
	BossRecallNPC(z *Boss, a Actor)

	// StadiumNotice tells player a it entered or left fighting stadium z;
	// leaving also closes its match window.
	StadiumNotice(z *Olympiad, a Actor, entering bool)
	// StadiumMatchInfo shows player a, entering z while its fight runs,
	// the match.
	StadiumMatchInfo(z *Olympiad, a Actor)
	// StadiumExpel sends a, a playable entering z, out unless its player
	// has business there.
	StadiumExpel(z *Olympiad, a Actor)

	// StartPulse begins the periodic task of effect or damage zone z. The
	// zone fires it at most once until the pulse stops and resets the
	// zone's latch; a call may overlap the end of the previous task.
	StartPulse(z Kind)

	// CastleBanish, HallBanish and SiegeBanish throw player a out of z as
	// z banishes foreigners; TeleportRoomEject sends player a, flushed out
	// of z, to to. They run on the command's own goroutine.
	CastleBanish(z *Castle, a Actor)
	HallBanish(z *ClanHall, a Actor)
	SiegeBanish(z *Siege, a Actor)
	TeleportRoomEject(z *CastleTeleport, a Actor, to location.Location)

	// StadiumFighting reports whether a match is fighting in stadium z.
	StadiumFighting(z *Olympiad) bool
	// CastleSiegeRunning reports whether the siege of castleID runs.
	CastleSiegeRunning(castleID int) bool
}

// Wire gives every zone of ix its effects. It runs before ix is shared.
func (ix *Index) Wire(fx Effects) {
	for _, k := range ix.set().all {
		k.Core().fx = fx
	}
}

// effects is what z's rules act through.
func (z *Zone) effects() Effects {
	if z.fx == nil {
		return NoEffects{}
	}
	return z.fx
}

// NoEffects carries out nothing: no match fights, no siege runs. A test
// double embeds it to carry out only the effects it watches; the server's
// own Effects implements every method, so a new effect does not compile
// until the server carries it out.
type NoEffects struct{}

func (NoEffects) ArenaNotice(*Arena, Actor, bool)                             {}
func (NoEffects) MotherTreeNotice(*MotherTree, Actor, int)                    {}
func (NoEffects) NoLandingDismount(*NoLanding, Actor, bool)                   {}
func (NoEffects) ShowHallInterior(*ClanHall, Actor)                           {}
func (NoEffects) SiegeNotice(*Siege, Actor, bool)                             {}
func (NoEffects) SiegeDismount(*Siege, Actor, bool)                           {}
func (NoEffects) SiegeFlagPvP(*Siege, Actor)                                  {}
func (NoEffects) SiegeUnsummon(*Siege, Actor)                                 {}
func (NoEffects) BossEject(*Boss, Actor)                                      {}
func (NoEffects) BossUnsummon(*Boss, Actor)                                   {}
func (NoEffects) BossRecallRaids(*Boss)                                       {}
func (NoEffects) BossRecallNPC(*Boss, Actor)                                  {}
func (NoEffects) StadiumNotice(*Olympiad, Actor, bool)                        {}
func (NoEffects) StadiumMatchInfo(*Olympiad, Actor)                           {}
func (NoEffects) StadiumExpel(*Olympiad, Actor)                               {}
func (NoEffects) StartPulse(Kind)                                             {}
func (NoEffects) CastleBanish(*Castle, Actor)                                 {}
func (NoEffects) HallBanish(*ClanHall, Actor)                                 {}
func (NoEffects) SiegeBanish(*Siege, Actor)                                   {}
func (NoEffects) TeleportRoomEject(*CastleTeleport, Actor, location.Location) {}
func (NoEffects) StadiumFighting(*Olympiad) bool                              { return false }
func (NoEffects) CastleSiegeRunning(int) bool                                 { return false }
