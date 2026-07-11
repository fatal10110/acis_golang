package zone

import (
	"github.com/fatal10110/acis_golang/internal/commons"
)

// Arena is a combat zone: everyone inside fights freely (no flag, no
// karma) and friend summoning is blocked.
type Arena struct {
	Zone
	// CombatNotice tells a player they entered or left a combat zone; nil
	// until the messaging layer wires it.
	CombatNotice func(a Actor, entering bool)
}

// NewArena builds an arena zone.
func NewArena(id int, form Form) *Arena { return &Arena{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Arena) Core() *Zone { return &z.Zone }

func (z *Arena) affects(Actor) bool { return true }

func (z *Arena) enter(a Actor) {
	if a.Class() == ClassPlayer && z.CombatNotice != nil {
		z.CombatNotice(a, true)
	}
	a.ZoneFlags().Set(FlagPvP, true)
	a.ZoneFlags().Set(FlagNoSummonFriend, true)
}

func (z *Arena) exit(a Actor) {
	a.ZoneFlags().Set(FlagPvP, false)
	a.ZoneFlags().Set(FlagNoSummonFriend, false)
	if a.Class() == ClassPlayer && z.CombatNotice != nil {
		z.CombatNotice(a, false)
	}
}

// DerbyTrack wraps the monster race track: playables inside are at peace,
// cannot summon friends, and carry the race-track marker.
type DerbyTrack struct{ Zone }

// NewDerbyTrack builds a race track zone.
func NewDerbyTrack(id int, form Form) *DerbyTrack { return &DerbyTrack{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *DerbyTrack) Core() *Zone { return &z.Zone }

func (z *DerbyTrack) affects(Actor) bool { return true }

func (z *DerbyTrack) enter(a Actor) {
	if a.Class().Playable() {
		a.ZoneFlags().Set(FlagMonsterTrack, true)
		a.ZoneFlags().Set(FlagPeace, true)
		a.ZoneFlags().Set(FlagNoSummonFriend, true)
	}
}

func (z *DerbyTrack) exit(a Actor) {
	if a.Class().Playable() {
		a.ZoneFlags().Set(FlagMonsterTrack, false)
		a.ZoneFlags().Set(FlagPeace, false)
		a.ZoneFlags().Set(FlagNoSummonFriend, false)
	}
}

// Fishing marks water where fishing is allowed; it imposes nothing on
// occupants.
type Fishing struct{ Zone }

// NewFishing builds a fishing zone.
func NewFishing(id int, form Form) *Fishing { return &Fishing{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Fishing) Core() *Zone { return &z.Zone }

func (z *Fishing) affects(Actor) bool { return true }
func (z *Fishing) enter(Actor)        {}
func (z *Fishing) exit(Actor)         {}

// WaterLevel is the water surface's z coordinate.
func (z *Fishing) WaterLevel() int { return z.form.HighZ() }

// HQ marks ground where siege headquarters may be built.
type HQ struct{ Zone }

// NewHQ builds a headquarters zone.
func NewHQ(id int, form Form) *HQ { return &HQ{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *HQ) Core() *Zone { return &z.Zone }

func (z *HQ) affects(Actor) bool { return true }

func (z *HQ) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagHQ, true)
	}
}

func (z *HQ) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagHQ, false)
	}
}

// Jail confines punished players: no friend summoning and no private
// stores inside.
type Jail struct{ Zone }

// NewJail builds a jail zone.
func NewJail(id int, form Form) *Jail { return &Jail{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Jail) Core() *Zone { return &z.Zone }

func (z *Jail) affects(Actor) bool { return true }

func (z *Jail) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagJail, true)
		a.ZoneFlags().Set(FlagNoSummonFriend, true)
		a.ZoneFlags().Set(FlagNoStore, true)
	}
}

func (z *Jail) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagJail, false)
		a.ZoneFlags().Set(FlagNoSummonFriend, false)
		a.ZoneFlags().Set(FlagNoStore, false)
	}
}

// NoLanding forbids wyvern landing; riders who linger get dismounted.
type NoLanding struct {
	Zone
	// DismountTimer starts (entering true) or cancels (entering false) the
	// forced-dismount countdown for a mounted player; nil until the mount
	// system wires it.
	DismountTimer func(a Actor, entering bool)
}

// NewNoLanding builds a no-landing zone.
func NewNoLanding(id int, form Form) *NoLanding { return &NoLanding{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *NoLanding) Core() *Zone { return &z.Zone }

func (z *NoLanding) affects(Actor) bool { return true }

func (z *NoLanding) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoLanding, true)
		if z.DismountTimer != nil {
			z.DismountTimer(a, true)
		}
	}
}

func (z *NoLanding) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoLanding, false)
		if z.DismountTimer != nil {
			z.DismountTimer(a, false)
		}
	}
}

// NoRestart forbids restarting (logging back in) inside its bounds.
type NoRestart struct{ Zone }

// NewNoRestart builds a no-restart zone.
func NewNoRestart(id int, form Form) *NoRestart { return &NoRestart{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *NoRestart) Core() *Zone { return &z.Zone }

func (z *NoRestart) affects(Actor) bool { return true }

func (z *NoRestart) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoRestart, true)
	}
}

func (z *NoRestart) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoRestart, false)
	}
}

// NoStore forbids private stores.
type NoStore struct{ Zone }

// NewNoStore builds a no-store zone.
func NewNoStore(id int, form Form) *NoStore { return &NoStore{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *NoStore) Core() *Zone { return &z.Zone }

func (z *NoStore) affects(Actor) bool { return true }

func (z *NoStore) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoStore, true)
	}
}

func (z *NoStore) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagNoStore, false)
	}
}

// NoSummonFriend blocks the friend-summoning skill for everyone inside.
type NoSummonFriend struct{ Zone }

// NewNoSummonFriend builds a no-summon zone.
func NewNoSummonFriend(id int, form Form) *NoSummonFriend {
	return &NoSummonFriend{Zone: newZone(id, form)}
}

// Core exposes the shared zone state.
func (z *NoSummonFriend) Core() *Zone { return &z.Zone }

func (z *NoSummonFriend) affects(Actor) bool { return true }
func (z *NoSummonFriend) enter(a Actor)      { a.ZoneFlags().Set(FlagNoSummonFriend, true) }
func (z *NoSummonFriend) exit(a Actor)       { a.ZoneFlags().Set(FlagNoSummonFriend, false) }

// Peace suspends all hostilities for everyone inside.
type Peace struct{ Zone }

// NewPeace builds a peace zone.
func NewPeace(id int, form Form) *Peace { return &Peace{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Peace) Core() *Zone { return &z.Zone }

func (z *Peace) affects(Actor) bool { return true }
func (z *Peace) enter(a Actor)      { a.ZoneFlags().Set(FlagPeace, true) }
func (z *Peace) exit(a Actor)       { a.ZoneFlags().Set(FlagPeace, false) }

// Prayer surrounds castle artifacts: casts aimed at the artifact must be
// made from inside it.
type Prayer struct{ Zone }

// NewPrayer builds an artifact prayer zone.
func NewPrayer(id int, form Form) *Prayer { return &Prayer{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Prayer) Core() *Zone { return &z.Zone }

func (z *Prayer) affects(Actor) bool { return true }
func (z *Prayer) enter(a Actor)      { a.ZoneFlags().Set(FlagCastOnArtifact, true) }
func (z *Prayer) exit(a Actor)       { a.ZoneFlags().Set(FlagCastOnArtifact, false) }

// Script marks ground that quests and custom scripts watch.
type Script struct{ Zone }

// NewScript builds a script zone.
func NewScript(id int, form Form) *Script { return &Script{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Script) Core() *Zone { return &z.Zone }

func (z *Script) affects(Actor) bool { return true }
func (z *Script) enter(a Actor)      { a.ZoneFlags().Set(FlagScript, true) }
func (z *Script) exit(a Actor)       { a.ZoneFlags().Set(FlagScript, false) }

// Water marks water volumes: occupants swim, and players below the
// surface too long start drowning.
type Water struct {
	Zone
	// SwimStateChanged switches the actor's movement into or out of swim
	// mode and rebroadcasts its appearance to observers; nil until the
	// movement layer wires it.
	SwimStateChanged func(a Actor, swimming bool)
}

// NewWater builds a water zone.
func NewWater(id int, form Form) *Water { return &Water{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Water) Core() *Zone { return &z.Zone }

func (z *Water) affects(Actor) bool { return true }

func (z *Water) enter(a Actor) {
	a.ZoneFlags().Set(FlagWater, true)
	if z.SwimStateChanged != nil {
		z.SwimStateChanged(a, true)
	}
}

func (z *Water) exit(a Actor) {
	a.ZoneFlags().Set(FlagWater, false)
	if z.SwimStateChanged != nil {
		z.SwimStateChanged(a, false)
	}
}

// WaterLevel is the water surface's z coordinate.
func (z *Water) WaterLevel() int { return z.form.HighZ() }

// MotherTree boosts HP/MP regeneration for players inside, optionally only
// for one race, with entry and exit announcements.
type MotherTree struct {
	Zone
	// EnterMessage and LeaveMessage are system message ids announced on
	// crossing; 0 means silent.
	EnterMessage int
	LeaveMessage int
	// HPBonus and MPBonus are flat regeneration bonuses granted inside.
	HPBonus int
	MPBonus int
	// AffectedRace limits the zone to one race ordinal; -1 affects all.
	AffectedRace int
	// Notify delivers a system message to a player; nil until the
	// messaging layer wires it.
	Notify func(a Actor, messageID int)
}

// NewMotherTree builds a regeneration zone from its data settings.
func NewMotherTree(id int, form Form, set *commons.StatSet) (*MotherTree, error) {
	enterMsg, err := set.GetIntDefault("enterMsgId", 0)
	if err != nil {
		return nil, err
	}
	leaveMsg, err := set.GetIntDefault("leaveMsgId", 0)
	if err != nil {
		return nil, err
	}
	mp, err := set.GetIntDefault("MpRegenBonus", 1)
	if err != nil {
		return nil, err
	}
	hp, err := set.GetIntDefault("HpRegenBonus", 1)
	if err != nil {
		return nil, err
	}
	race, err := set.GetIntDefault("affectedRace", -1)
	if err != nil {
		return nil, err
	}
	return &MotherTree{
		Zone:         newZone(id, form),
		EnterMessage: enterMsg,
		LeaveMessage: leaveMsg,
		HPBonus:      hp,
		MPBonus:      mp,
		AffectedRace: race,
	}, nil
}

// Core exposes the shared zone state.
func (z *MotherTree) Core() *Zone { return &z.Zone }

func (z *MotherTree) affects(a Actor) bool {
	if z.AffectedRace > -1 {
		if p, ok := a.(Player); ok && a.Class() == ClassPlayer {
			return int(p.Race()) == z.AffectedRace
		}
	}
	return true
}

func (z *MotherTree) enter(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagMotherTree, true)
		if z.EnterMessage != 0 && z.Notify != nil {
			z.Notify(a, z.EnterMessage)
		}
	}
}

func (z *MotherTree) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagMotherTree, false)
		if z.LeaveMessage != 0 && z.Notify != nil {
			z.Notify(a, z.LeaveMessage)
		}
	}
}

// Town rules over a town: peace by default, taxation tied to a castle.
type Town struct {
	Zone
	// TownID is the town's map id.
	TownID int
	// CastleID links the town to the castle collecting its taxes; 0 for
	// castle-free towns.
	CastleID int
	// Peaceful reports whether the town suspends hostilities (the data
	// default; special event towns may disable it).
	Peaceful bool
	// CombatRule adjusts town combat handling: 0 applies peace normally,
	// 1 keeps siege participants flagged (needs InSiege), 2 disables the
	// peace flag townwide.
	CombatRule int
	// InSiege reports whether the player currently takes part in a siege;
	// consulted only under CombatRule 1. Nil means no siege participation.
	InSiege func(a Actor) bool
}

// NewTown builds a town zone from its data settings.
func NewTown(id int, form Form, set *commons.StatSet) (*Town, error) {
	townID, err := set.GetIntDefault("townId", 0)
	if err != nil {
		return nil, err
	}
	castleID, err := set.GetIntDefault("castleId", 0)
	if err != nil {
		return nil, err
	}
	peaceful := set.GetBoolDefault("isPeaceZone", true)
	return &Town{Zone: newZone(id, form), TownID: townID, CastleID: castleID, Peaceful: peaceful}, nil
}

// Core exposes the shared zone state.
func (z *Town) Core() *Zone { return &z.Zone }

func (z *Town) affects(Actor) bool { return true }

func (z *Town) enter(a Actor) {
	if z.CombatRule == 1 && a.Class() == ClassPlayer && z.InSiege != nil && z.InSiege(a) {
		return
	}
	if z.Peaceful && z.CombatRule != 2 {
		a.ZoneFlags().Set(FlagPeace, true)
	}
	a.ZoneFlags().Set(FlagTown, true)
}

func (z *Town) exit(a Actor) {
	if z.Peaceful {
		a.ZoneFlags().Set(FlagPeace, false)
	}
	a.ZoneFlags().Set(FlagTown, false)
}
