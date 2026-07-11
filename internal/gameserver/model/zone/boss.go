package zone

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Boss guards a raid boss lair. Entry is restricted to an allow list with
// per-player deadlines: players may re-enter after a disconnect within the
// invade window, anyone else is thrown out.
type Boss struct {
	Zone
	// InvadeWindow is how long a permitted player may (re-)enter; zero
	// disables all entry policing.
	InvadeWindow time.Duration
	// OustLoc is where rejected players land; the zero value means the
	// player's town respawn instead.
	OustLoc location.Location

	// Eject teleports a rejected player out (to OustLoc when set, town
	// respawn otherwise); nil until the teleport system wires it.
	Eject func(a Actor)
	// Unsummon dismisses a summon whose owner has no entry permission;
	// nil until the summon system wires it.
	Unsummon func(a Actor)
	// RecallRaids sends every raid-related monster inside back to its
	// spawn once the last playable leaves; nil until the AI system wires
	// it.
	RecallRaids func()
	// RecallNPC sends one departing monster home if it is raid-related;
	// nil until the AI system wires it.
	RecallNPC func(a Actor)

	// now is the clock used for deadlines; test seam, defaults to
	// time.Now.
	now func() time.Time

	// bmu guards allowed and deadlines.
	bmu       sync.Mutex
	allowed   map[int32]struct{}
	deadlines map[int32]time.Time
}

// NewBoss builds a boss lair zone from its data settings.
func NewBoss(id int, form Form, set *commons.StatSet) (*Boss, error) {
	f := commons.NewFields(set, "zone: boss")
	invade := f.IntDefault("InvadeTime", 0)
	oustX := f.IntDefault("oustX", 0)
	oustY := f.IntDefault("oustY", 0)
	oustZ := f.IntDefault("oustZ", 0)
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &Boss{
		Zone:         newZone(id, form),
		InvadeWindow: time.Duration(invade) * time.Millisecond,
		OustLoc:      location.Location{X: oustX, Y: oustY, Z: oustZ},
		now:          time.Now,
		allowed:      make(map[int32]struct{}),
		deadlines:    make(map[int32]time.Time),
	}, nil
}

// Core exposes the shared zone state.
func (z *Boss) Core() *Zone { return &z.Zone }

func (z *Boss) affects(Actor) bool { return true }

func (z *Boss) enter(a Actor) {
	a.ZoneFlags().Set(FlagBoss, true)

	switch a.Class() {
	case ClassPlayer:
		a.ZoneFlags().Set(FlagNoSummonFriend, true)

		p, isPlayer := a.(Player)
		if (isPlayer && p.GM()) || z.InvadeWindow == 0 {
			return
		}
		if z.consumeEntry(a.ObjectID()) {
			return
		}
		if z.Eject != nil {
			z.Eject(a)
		}
	case ClassSummon:
		o, ok := a.(Owned)
		if !ok {
			return
		}
		owner, ok := o.Owner()
		if !ok {
			return
		}
		if z.isAllowed(owner.ObjectID()) || owner.GM() || z.InvadeWindow == 0 {
			return
		}
		if z.Unsummon != nil {
			z.Unsummon(a)
		}
	}
}

// consumeEntry checks and spends the one-shot entry deadline for id: a
// permitted player whose deadline has not passed enters (the deadline is
// consumed); an expired or missing deadline revokes the permission.
func (z *Boss) consumeEntry(id int32) bool {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	if _, ok := z.allowed[id]; !ok {
		return false
	}
	deadline, ok := z.deadlines[id]
	delete(z.deadlines, id)
	if ok && deadline.After(z.now()) {
		return true
	}
	delete(z.allowed, id)
	return false
}

func (z *Boss) exit(a Actor) {
	a.ZoneFlags().Set(FlagBoss, false)

	if a.Class().Playable() {
		if a.Class() == ClassPlayer {
			a.ZoneFlags().Set(FlagNoSummonFriend, false)

			p, isPlayer := a.(Player)
			if (isPlayer && p.GM()) || z.InvadeWindow == 0 {
				return
			}
			if z.settleExit(a.ObjectID(), isPlayer && p.Online()) {
				return
			}
		}

		// Once the last playable is gone, send lingering raid monsters
		// home.
		if z.hasOccupants() && !z.hasPlayableInside() && z.RecallRaids != nil {
			z.RecallRaids()
		}
	} else if z.RecallNPC != nil {
		z.RecallNPC(a)
	}
}

// settleExit updates the allow list when a permitted player leaves: a
// disconnect grants a re-entry deadline, a plain walk-out revokes the
// permission unless a deadline is already pending. It reports whether the
// pending-deadline case applied, which also stops the rest of the exit
// handling (the raid recall check does not run for that player).
func (z *Boss) settleExit(id int32, online bool) (stop bool) {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	if _, ok := z.allowed[id]; !ok {
		return false
	}
	if !online {
		z.deadlines[id] = z.now().Add(z.InvadeWindow)
		return false
	}
	if _, pending := z.deadlines[id]; pending {
		return true
	}
	delete(z.allowed, id)
	return false
}

func (z *Boss) hasOccupants() bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return len(z.occupants) > 0
}

func (z *Boss) hasPlayableInside() bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	for _, a := range z.occupants {
		if a.Class().Playable() {
			return true
		}
	}
	return false
}

// isAllowed reports whether id currently holds an entry permission.
func (z *Boss) isAllowed(id int32) bool {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	_, ok := z.allowed[id]
	return ok
}

// AllowEntry permits the player to enter (or re-enter) for the given
// window from now. Boot-time restores use the zone's own InvadeWindow.
func (z *Boss) AllowEntry(id int32, window time.Duration) {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	z.allowed[id] = struct{}{}
	z.deadlines[id] = z.now().Add(window)
}

// RevokeEntry withdraws the player's entry permission and any pending
// deadline.
func (z *Boss) RevokeEntry(id int32) {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	delete(z.allowed, id)
	delete(z.deadlines, id)
}

// AllowedPlayers snapshots the ids currently holding entry permission,
// for persistence across restarts.
func (z *Boss) AllowedPlayers() []int32 {
	z.bmu.Lock()
	defer z.bmu.Unlock()
	out := make([]int32, 0, len(z.allowed))
	for id := range z.allowed {
		out = append(out, id)
	}
	return out
}

// OustAll throws every online player inside out via the Eject hook and
// wipes all entry permissions. It returns the players it acted on.
func (z *Boss) OustAll() []Actor {
	players := z.playersInside(nil)
	for _, a := range players {
		p, ok := a.(Player)
		if ok && !p.Online() {
			continue
		}
		if z.Eject != nil {
			z.Eject(a)
		}
	}
	z.bmu.Lock()
	z.allowed = make(map[int32]struct{})
	z.deadlines = make(map[int32]time.Time)
	z.bmu.Unlock()
	return players
}
