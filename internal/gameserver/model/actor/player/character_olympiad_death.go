package player

// summonIdler is c's summon as an Olympiad fall sends it idle.
type summonIdler interface {
	TryToIdle()
}

// fallInOlympiad is the whole death of a competitor whose HP damage emptied
// its HP: it stops its movement, attack and cast while still standing,
// keeping its target, then is marked dead with its regeneration stopped,
// and its summon, if one is out, goes idle. Nothing else of a death runs:
// no death packet, no kill credit, no death penalty, no drop, no effect
// strip and no further HP write. The match reads the dead mark to end the
// fight, and the stadium lifts it without a revive when the fight ends
// (ClearOlympiadDeath). It reports whether this call marked c dead.
//
// falling makes the fall once-only: a second lethal hit landing while the
// first one's fall runs, or after it, finds the fall claimed or c dead and
// does nothing.
func (c *Character) fallInOlympiad() bool {
	if !c.olympiad.falling.CompareAndSwap(false, true) {
		return false
	}
	defer c.olympiad.falling.Store(false)
	if c.Dead() {
		return false
	}
	if c.sink == nil {
		c.StopCast()
	}
	c.AbortAll(false)
	if !c.markDead(false) {
		return false
	}
	c.settleRegenTask()
	if obj, ok := c.Summon(); ok {
		if s, ok := obj.(summonIdler); ok {
			s.TryToIdle()
		}
	}
	return true
}
