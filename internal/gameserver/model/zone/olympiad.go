package zone

// Olympiad is a tournament stadium: restart and friend summoning are
// blocked, and while a match runs the ring is a combat zone. Only match
// participants, observers and game masters may stay.
type Olympiad struct {
	Zone
	Spawns

	// BattleStarted reports whether a match is currently running in this
	// stadium; nil (until the tournament system wires it) reads as no
	// match.
	BattleStarted func() bool
	// CombatNotice tells a player they entered or left the active ring
	// (leaving also closes the match scoreboard); nil until the messaging
	// layer wires it.
	CombatNotice func(a Actor, entering bool)
	// SendMatchInfo shows an entering player the running match's state;
	// nil until the tournament system wires it.
	SendMatchInfo func(a Actor)
	// ExpelUninvited unsummons and teleports out an entering player-side
	// actor whose controlling player is neither participant, observer nor
	// game master; nil until the tournament system wires it.
	ExpelUninvited func(a Actor)
}

// NewOlympiad builds a stadium zone.
func NewOlympiad(id int, form Form) *Olympiad { return &Olympiad{Zone: newZone(id, form)} }

// Core exposes the shared zone state.
func (z *Olympiad) Core() *Zone { return &z.Zone }

func (z *Olympiad) affects(Actor) bool { return true }

func (z *Olympiad) battleRunning() bool { return z.BattleStarted != nil && z.BattleStarted() }

func (z *Olympiad) enter(a Actor) {
	a.ZoneFlags().Set(FlagNoSummonFriend, true)
	a.ZoneFlags().Set(FlagNoRestart, true)

	if z.battleRunning() {
		a.ZoneFlags().Set(FlagPvP, true)
		if a.Class() == ClassPlayer {
			if z.CombatNotice != nil {
				z.CombatNotice(a, true)
			}
			if z.SendMatchInfo != nil {
				z.SendMatchInfo(a)
			}
		}
	}

	// Anything player-controlled answers to a player who must be allowed
	// to be here; the tournament system makes the call.
	if a.Class().Playable() && z.ExpelUninvited != nil {
		z.ExpelUninvited(a)
	}
}

func (z *Olympiad) exit(a Actor) {
	a.ZoneFlags().Set(FlagNoSummonFriend, false)
	a.ZoneFlags().Set(FlagNoRestart, false)

	if z.battleRunning() {
		a.ZoneFlags().Set(FlagPvP, false)
		if a.Class() == ClassPlayer && z.CombatNotice != nil {
			z.CombatNotice(a, false)
		}
	}
}

// UpdateCombatStatus re-syncs every occupant's combat state with the
// stadium's match state: called when a match starts or ends while people
// are already inside.
func (z *Olympiad) UpdateCombatStatus() {
	running := z.battleRunning()
	for _, a := range z.Occupants() {
		if running {
			a.ZoneFlags().Set(FlagPvP, true)
			if a.Class() == ClassPlayer && z.CombatNotice != nil {
				z.CombatNotice(a, true)
			}
		} else {
			a.ZoneFlags().Set(FlagPvP, false)
			if a.Class() == ClassPlayer && z.CombatNotice != nil {
				z.CombatNotice(a, false)
			}
		}
	}
}
