package zone

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Siege covers a besiegeable residence's battlefield. FlagSiege marks raw
// membership in the battlefield geometry regardless of whether a siege is
// running, matching the reference's isInsideZone(SIEGE) check. While a
// siege is actually running, everyone inside is additionally in open
// combat, cannot summon friends, and flying players are grounded; leaving
// the battlefield then flags the player for pvp.
type Siege struct {
	Zone
	// ResidenceID is the besiegeable residence (castle or clan hall) the
	// battlefield belongs to; -1 when unset.
	ResidenceID int

	// CombatNotice tells a player they entered or left the combat zone;
	// nil until the messaging layer wires it.
	CombatNotice func(a Actor, entering bool)
	// DismountTimer starts (entering true) or cancels (entering false)
	// the forced-dismount countdown for a flying player; nil until the
	// mount system wires it.
	DismountTimer func(a Actor, entering bool)
	// FlagPvP puts a leaving player on the timed pvp flag; nil until the
	// pvp system wires it.
	FlagPvP func(a Actor)
	// Unsummon dismisses a siege-bound summon that leaves or loses its
	// battlefield; nil until the summon system wires it.
	Unsummon func(a Actor)
	// Banish teleports a player to their town respawn when thrown off the
	// battlefield; nil until the teleport system wires it.
	Banish func(a Actor)

	active atomic.Bool
}

// NewSiege builds a siege battlefield zone from its data settings.
func NewSiege(id int, form Form, set *commons.StatSet) (*Siege, error) {
	f := commons.NewFields(set, "zone: siege")
	residence := -1
	for _, key := range []string{"castleId", "clanHallId"} {
		if f.Has(key) {
			residence = f.Int(key)
		}
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &Siege{Zone: newZone(id, form), ResidenceID: residence}, nil
}

// Core exposes the shared zone state.
func (z *Siege) Core() *Zone { return &z.Zone }

func (z *Siege) affects(Actor) bool { return true }

// Active reports whether the siege is currently running.
func (z *Siege) Active() bool {
	return z.active.Load()
}

func (z *Siege) enter(a Actor) {
	a.ZoneFlags().Set(FlagSiege, true)
	if z.Active() {
		z.applyCombatEntry(a)
	}
}

// applyCombatEntry imposes the active-siege combat state on an occupant.
// Callable both from enter (fresh membership, siege already active) and
// from SetActive(true) (existing occupants when the siege starts).
func (z *Siege) applyCombatEntry(a Actor) {
	a.ZoneFlags().Set(FlagPvP, true)
	a.ZoneFlags().Set(FlagNoSummonFriend, true)
	if a.Class() == ClassPlayer {
		if z.CombatNotice != nil {
			z.CombatNotice(a, true)
		}
		if z.DismountTimer != nil {
			z.DismountTimer(a, true)
		}
	}
}

func (z *Siege) exit(a Actor) {
	a.ZoneFlags().Set(FlagPvP, false)
	a.ZoneFlags().Set(FlagSiege, false)
	a.ZoneFlags().Set(FlagNoSummonFriend, false)
	switch a.Class() {
	case ClassPlayer:
		if z.Active() {
			if z.CombatNotice != nil {
				z.CombatNotice(a, false)
			}
			if z.DismountTimer != nil {
				z.DismountTimer(a, false)
			}
			if z.FlagPvP != nil {
				z.FlagPvP(a)
			}
		}
	case ClassSummon:
		if z.Unsummon != nil {
			z.Unsummon(a)
		}
	}
}

// SetActive switches the siege on or off. Turning it on imposes the combat
// rules on everyone already inside (their FlagSiege membership is
// untouched, they never left the battlefield); turning it off strips the
// combat state from them (without the leave-battlefield pvp flag) while
// leaving FlagSiege set, since they are still standing in the battlefield.
func (z *Siege) SetActive(v bool) {
	z.active.Store(v)

	if v {
		for _, a := range z.Occupants() {
			z.applyCombatEntry(a)
		}
		return
	}
	for _, a := range z.Occupants() {
		a.ZoneFlags().Set(FlagPvP, false)
		a.ZoneFlags().Set(FlagNoSummonFriend, false)
		switch a.Class() {
		case ClassPlayer:
			if z.CombatNotice != nil {
				z.CombatNotice(a, false)
			}
			if z.DismountTimer != nil {
				z.DismountTimer(a, false)
			}
		case ClassSummon:
			if z.Unsummon != nil {
				z.Unsummon(a)
			}
		}
	}
}

// BanishForeigners throws every player not belonging to clanID out of the
// battlefield, via the Banish hook.
func (z *Siege) BanishForeigners(clanID int32) {
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
