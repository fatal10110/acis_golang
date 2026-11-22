package zone

import (
	"sync/atomic"
)

// Siege covers a besiegeable residence's battlefield. While a siege runs,
// everyone inside is in open combat, cannot summon friends, and flying
// players are grounded; leaving the battlefield flags the player for pvp.
type Siege struct {
	Zone
	// ResidenceID is the besiegeable residence (castle or clan hall) the
	// battlefield belongs to; -1 when unset.
	ResidenceID int

	active atomic.Bool
}

// NewSiege builds an inactive siege battlefield zone not yet tied to a
// residence; the loader sets ResidenceID from the zone's data settings
// before publishing it.
func NewSiege(id int, form Form) *Siege {
	return &Siege{Zone: newZone(id, form), ResidenceID: -1}
}

// Core exposes the shared zone state.
func (z *Siege) Core() *Zone { return &z.Zone }

func (z *Siege) affects(Actor) bool { return true }

// Active reports whether the siege is currently running.
func (z *Siege) Active() bool {
	return z.active.Load()
}

// setBattlefieldHolds raises (on) or releases the holds a running siege
// keeps on everyone inside: open combat, the siege zone, and no summoning of
// friends.
func setBattlefieldHolds(a Actor, on bool) {
	a.ZoneFlags().Set(FlagPvP, on)
	a.ZoneFlags().Set(FlagSiege, on)
	a.ZoneFlags().Set(FlagNoSummonFriend, on)
}

func (z *Siege) enter(a Actor) {
	if z.raise(a) {
		z.combatEntered(a)
	}
}

// raise raises a's battlefield holds when the siege runs and a, still
// inside, holds none yet, reporting whether it did.
func (z *Siege) raise(a Actor) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.active.Load() && z.switchOnLocked(a, setBattlefieldHolds)
}

// combatEntered tells a player its battlefield holds were just raised.
func (z *Siege) combatEntered(a Actor) {
	if a.Class() != ClassPlayer {
		return
	}
	z.effects().SiegeNotice(z, a, true)
	z.effects().SiegeDismount(z, a, true)
}

func (z *Siege) exit(a Actor) {
	z.mu.Lock()
	// Leaving releases the battlefield holds once, whether or not the siege
	// raised them.
	if !z.switchOffLocked(a, setBattlefieldHolds) {
		setBattlefieldHolds(a, false)
	}
	active := z.active.Load()
	z.mu.Unlock()
	switch a.Class() {
	case ClassPlayer:
		if active {
			z.effects().SiegeNotice(z, a, false)
			z.effects().SiegeDismount(z, a, false)
			z.effects().SiegeFlagPvP(z, a)
		}
	case ClassSummon:
		z.effects().SiegeUnsummon(z, a)
	}
}

// SetActive switches the siege on or off. Turning it on replays the entry
// rules for everyone already inside; turning it off strips the combat
// state from them (without the leave-battlefield pvp flag). Each occupant
// is replayed under its own zone membership.
func (z *Siege) SetActive(v bool) {
	z.mu.Lock()
	z.active.Store(v)
	z.mu.Unlock()
	for _, a := range z.Occupants() {
		replayOn(a, func() { z.replay(a) })
	}
}

// replay brings a's battlefield holds in line with whether the siege runs
// now: raised, with the entry notices, for an occupant holding none while
// it runs; released for one holding them once it stopped, every occupant
// then hearing it left the combat zone and a summon dismissed.
func (z *Siege) replay(a Actor) {
	if z.raise(a) {
		z.combatEntered(a)
		return
	}
	z.mu.Lock()
	off := !z.active.Load()
	if off {
		z.switchOffLocked(a, setBattlefieldHolds)
	}
	_, inside := z.occupants[a.ObjectID()]
	z.mu.Unlock()
	if !off || !inside {
		return
	}
	switch a.Class() {
	case ClassPlayer:
		z.effects().SiegeNotice(z, a, false)
		z.effects().SiegeDismount(z, a, false)
	case ClassSummon:
		z.effects().SiegeUnsummon(z, a)
	}
}

// BanishForeigners throws every player not belonging to clanID out of the
// battlefield, through its effects.
func (z *Siege) BanishForeigners(clanID int32) {
	for _, a := range z.playersInside(func(a Actor) bool {
		p, ok := a.(Player)
		return ok && p.ClanID() != clanID
	}) {
		z.effects().SiegeBanish(z, a)
	}
}
