package fishing

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
)

// Stance is one player's fishing: the line it cast, the fish on the other
// end and, once that fish bites, the fight with it. Its owner serializes
// every call; Stance holds no lock.
type Stance struct {
	night   bool
	fish    fish.Fish
	hasFish bool

	looking  bool
	deadline time.Time

	fighting  bool
	time      int
	stop      int
	goodUse   int
	anim      int
	mode      int
	deceptive int
	fishHP    int
	upper     bool
	lureType  int
}

// Fishing reports whether the stance waits for a bite or fights a fish.
func (s *Stance) Fishing() bool { return s.looking || s.fighting }

// Fighting reports whether a hooked fish is being fought.
func (s *Stance) Fighting() bool { return s.fighting }

// HasFish reports whether a fish was drawn for the line now cast.
func (s *Stance) HasFish() bool { return s.hasFish }

// Fish returns the fish drawn for the line now cast.
func (s *Stance) Fish() fish.Fish { return s.fish }

// Cast puts a line baited with lureID in the water. The fish it is cast
// for is set by Hook.
func (s *Stance) Cast(lureID int32) {
	s.night = NightLure(lureID)
}

// Hook sets f as the fish the line is cast for.
func (s *Stance) Hook(f fish.Fish) {
	s.fish = f
	s.hasFish = true
}

// FishType is the fish type the client shows for the line: the fish's own
// type, or -1 for a night lure cast while it is not night.
func (s *Stance) FishType(night bool) int {
	if s.night && !night {
		return -1
	}
	return s.fish.Type
}

// NightLure reports whether the line is baited with a night lure.
func (s *Stance) NightLure() bool { return s.night }

// Wait starts the wait for a bite, cast at now: a line still without a bite
// FirstLookDelay plus the fish's wait time later is reeled in empty.
func (s *Stance) Wait(now time.Time) {
	s.looking = true
	s.deadline = now.Add(FirstLookDelay + time.Duration(s.fish.WaitTime)*time.Millisecond)
}

// LookKind is what one look for a bite found.
type LookKind uint8

const (
	// LookNothing means no bite yet.
	LookNothing LookKind = iota
	// LookTimeout means the wait ran out: the line is reeled in empty.
	LookTimeout
	// LookBite means the fish bit: the fight starts as Combat says.
	LookBite
)

// Combat is how a fight starts: its time in seconds, the fish's HP, its
// first mode, the lure kind (0 beginner, 1 normal, 2 luminous) and whether
// the fish fights deceptively.
type Combat struct {
	Time, HP, Mode, LureType, Deceptive int
}

// Look looks once for a bite at now, night telling whether it is night.
// A night lure gets no bite by day. A bite starts the fight.
func (s *Stance) Look(now time.Time, night bool, roll Roll) (LookKind, Combat) {
	if !s.looking {
		return LookNothing, Combat{}
	}
	if !now.Before(s.deadline) {
		return LookTimeout, Combat{}
	}
	if s.FishType(night) == -1 {
		return LookNothing, Combat{}
	}
	if s.fish.Guts <= roll(1000) {
		return LookNothing, Combat{}
	}
	s.looking = false
	s.fighting = true
	s.fishHP = s.fish.HP
	s.time = s.fish.CombatTime / 1000
	s.upper = s.fish.Group == GroupUpper
	if s.upper {
		s.deceptive = 0
		if roll(100) >= 90 {
			s.deceptive = 1
		}
		s.lureType = 2
	} else {
		s.deceptive = 0
		s.lureType = 1
		if s.fish.Group == GroupBeginner {
			s.lureType = 0
		}
	}
	s.mode = 0
	if roll(100) >= 80 {
		s.mode = 1
	}
	return LookBite, Combat{Time: s.time, HP: s.fish.HP, Mode: s.mode, LureType: s.lureType, Deceptive: s.deceptive}
}

// HPRegen is one update of the fight gauge: time left in seconds, the
// fish's HP, its mode (0 resting, 1 fighting), the last action's result (0
// none, 1 success, 2 failure), its animation (0 none, 1 pumping, 2
// reeling), the penalty taken and whether the fish fights deceptively.
type HPRegen struct {
	Time, HP, Mode, GoodUse, Anim, Penalty, Deceptive int
}

// TickKind is what one second of the fight did.
type TickKind uint8

const (
	// TickGauge means the fight goes on: Gauge is the update to show.
	TickGauge TickKind = iota
	// TickStolen means the fish got its HP back to twice its maximum and
	// stole the bait.
	TickStolen
	// TickSpat means the fight's time ran out and the fish spat the hook.
	TickSpat
)

// Tick runs one second of the fight. A gauge update reaches watchers
// (broadcast) only while an action's animation is pending.
func (s *Stance) Tick(roll Roll) (kind TickKind, gauge HPRegen, broadcast bool) {
	if s.fishHP >= s.fish.HP*2 {
		return TickStolen, HPRegen{}, false
	}
	if s.time <= 0 {
		return TickSpat, HPRegen{}, false
	}
	s.time--
	if (s.mode == 1) == (s.deceptive == 0) {
		s.fishHP += s.fish.HPRegen
	}
	if s.stop == 0 {
		s.stop = 1
		if roll(100) >= 70 {
			s.mode ^= 1
		}
		if s.upper && roll(100) >= 90 {
			s.deceptive ^= 1
		}
	} else {
		s.stop--
	}
	return TickGauge, HPRegen{Time: s.time, HP: s.fishHP, Mode: s.mode, Anim: s.anim, Deceptive: s.deceptive}, s.anim != 0
}

// ActionMessage is the message a pumping or reeling action answers with.
type ActionMessage uint8

const (
	// ActionResisted means the fish resisted the attempt to bring it in.
	ActionResisted ActionMessage = iota
	// ActionPumped and ActionReeled mean the action took Damage off the
	// fish.
	ActionPumped
	ActionReeled
	// ActionPumpFailed and ActionReelFailed mean the fish regained Damage
	// HP.
	ActionPumpFailed
	ActionReelFailed
)

// End is how an action left the fight.
type End uint8

const (
	// EndNone means the fight goes on.
	EndNone End = iota
	// EndCaught means the fish's HP reached 0: it is caught.
	EndCaught
	// EndLost means the fish's HP went past twice its maximum: it got away.
	EndLost
)

// Action is the outcome of one pumping or reeling action: its message, the
// damage it names, whether the penalty notice follows it, the gauge update
// every watcher sees and whether it ended the fight.
type Action struct {
	Message       ActionMessage
	Damage        int
	PenaltyNotice bool
	Gauge         HPRegen
	End           End
}

// Pump pumps the hooked fish for damage, penalty already taken off. It
// succeeds while the fish rests, or fights while deceiving.
func (s *Stance) Pump(damage, penalty int, roll Roll) Action {
	return s.act(1, s.mode == 0, ActionPumped, ActionPumpFailed, damage, penalty, roll)
}

// Reel reels the hooked fish in for damage, penalty already taken off. It
// succeeds while the fish fights, or rests while deceiving.
func (s *Stance) Reel(damage, penalty int, roll Roll) Action {
	return s.act(2, s.mode == 1, ActionReeled, ActionReelFailed, damage, penalty, roll)
}

func (s *Stance) act(anim int, rightMode bool, success, failure ActionMessage, damage, penalty int, roll Roll) Action {
	s.anim = anim
	if roll(100) > 90 {
		s.goodUse = 0
		return s.changeHP(Action{Message: ActionResisted}, 0, penalty)
	}
	if rightMode == (s.deceptive == 0) {
		s.goodUse = 1
		return s.changeHP(Action{Message: success, Damage: damage, PenaltyNotice: penalty == expertisePenalty}, damage, penalty)
	}
	s.goodUse = 2
	return s.changeHP(Action{Message: failure, Damage: damage}, -damage, penalty)
}

// changeHP takes hp off the fish and fills in a's gauge update and end.
func (s *Stance) changeHP(a Action, hp, penalty int) Action {
	s.fishHP = max(s.fishHP-hp, 0)
	a.Gauge = HPRegen{Time: s.time, HP: s.fishHP, Mode: s.mode, GoodUse: s.goodUse, Anim: s.anim, Penalty: penalty, Deceptive: s.deceptive}
	s.anim = 0
	switch {
	case s.fishHP > s.fish.HP*2:
		s.fishHP = s.fish.HP * 2
		a.End = EndLost
	case s.fishHP == 0:
		a.End = EndCaught
	}
	return a
}

// Reset clears the stance once the line is out of the water.
func (s *Stance) Reset() {
	*s = Stance{}
}
