package zone

// Olympiad is a tournament stadium: restart and friend summoning are
// blocked, and while a match runs the ring is a combat zone. Only match
// participants, observers and game masters may stay.
type Olympiad struct {
	Zone
	Spawns

	// BattleStarted reports whether a match is currently fighting in this
	// stadium; nil reads as no match.
	BattleStarted func() bool
	// CombatNotice tells a player they entered or left the active ring
	// (leaving also closes the match scoreboard); nil tells nothing.
	CombatNotice func(a Actor, entering bool)
	// SendMatchInfo shows an entering player the running match's state;
	// nil shows nothing.
	SendMatchInfo func(a Actor)
	// ExpelUninvited unsummons and teleports out an entering player-side
	// actor whose controlling player is neither participant, observer nor
	// game master; nil expels no one.
	ExpelUninvited func(a Actor)
}

// NewOlympiad builds a stadium zone.
func NewOlympiad(id int, form Form) *Olympiad { return &Olympiad{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Olympiad) Core() *Zone { return &z.Zone }

func (z *Olympiad) affects(Actor) bool { return true }

func (z *Olympiad) battleRunning() bool { return z.BattleStarted != nil && z.BattleStarted() }

// setRingHolds raises (on) or releases the open combat a fighting stadium
// keeps everyone inside in.
func setRingHolds(a Actor, on bool) { a.ZoneFlags().Set(FlagPvP, on) }

func (z *Olympiad) enter(a Actor) {
	a.ZoneFlags().Set(FlagNoSummonFriend, true)
	a.ZoneFlags().Set(FlagNoRestart, true)

	if z.battleRunning() && z.raise(a) && a.Class() == ClassPlayer {
		if z.CombatNotice != nil {
			z.CombatNotice(a, true)
		}
		if z.SendMatchInfo != nil {
			z.SendMatchInfo(a)
		}
	}

	// Anything player-controlled answers to a player who must be allowed
	// to be here; the tournament system makes the call.
	if a.Class().Playable() && z.ExpelUninvited != nil {
		z.ExpelUninvited(a)
	}
}

// raise puts a, still inside and not yet in combat here, in the ring's
// combat, reporting whether it did.
func (z *Olympiad) raise(a Actor) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.switchOnLocked(a, setRingHolds)
}

// release takes a out of the ring's combat when it is in it, reporting
// whether it was.
func (z *Olympiad) release(a Actor) bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.switchOffLocked(a, setRingHolds)
}

func (z *Olympiad) exit(a Actor) {
	a.ZoneFlags().Set(FlagNoSummonFriend, false)
	a.ZoneFlags().Set(FlagNoRestart, false)

	released := z.release(a)
	if !z.battleRunning() {
		return
	}
	// A fight running releases the ring's combat on the way out even when
	// the stadium did not raise it.
	if !released {
		setRingHolds(a, false)
	}
	if a.Class() == ClassPlayer && z.CombatNotice != nil {
		z.CombatNotice(a, false)
	}
}

// UpdateCombatStatus re-syncs every occupant's combat state with the
// stadium's match state: called when a match starts or ends while people
// are already inside. Each occupant is replayed under its own zone
// membership. A start tells only the players it put in combat; a stop
// releases only the holds the stadium raised but tells every player still
// inside it left the combat zone, closing the scoreboard even when the
// match ended before the fight.
func (z *Olympiad) UpdateCombatStatus() {
	for _, a := range z.Occupants() {
		replayOn(a, func() { z.replay(a) })
	}
}

// replay brings a's ring combat in line with whether the fight runs now.
func (z *Olympiad) replay(a Actor) {
	if z.battleRunning() {
		if z.raise(a) && a.Class() == ClassPlayer && z.CombatNotice != nil {
			z.CombatNotice(a, true)
		}
		return
	}
	z.mu.Lock()
	z.switchOffLocked(a, setRingHolds)
	_, inside := z.occupants[a.ObjectID()]
	z.mu.Unlock()
	if inside && a.Class() == ClassPlayer && z.CombatNotice != nil {
		z.CombatNotice(a, false)
	}
}
