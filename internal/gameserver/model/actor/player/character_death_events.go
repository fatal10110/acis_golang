package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// DeathEvents takes the deaths of characters and of their summons.
type DeathEvents interface {
	// PlayableDied hands the death of victim, c itself or c's summon,
	// killed by killer (nil when no creature caused it), to the quests of
	// c's journal that react to their player's death.
	PlayableDied(c *Character, victim, killer attackable.Combatant)
}

// notifyDeath hands the death of victim, c or its summon, to c's death
// events. It runs on the goroutine running the death, which must hold none
// of c's locks.
func (c *Character) notifyDeath(victim, killer attackable.Combatant) {
	if c.deaths != nil {
		c.deaths.PlayableDied(c, victim, killer)
	}
}

// combatant returns c as the world tracks it: the object that embeds c
// when there is one.
func (c *Character) combatant() attackable.Combatant {
	if t, ok := c.tracked().(attackable.Combatant); ok {
		return t
	}
	return c
}

// SummonDied tells c that its summon died, killed by killer (nil when no
// creature caused it): the summon's death reaches c's quests as c's own
// does. The summon's death sequence calls it once observers have seen the
// death and before killer is credited for the kill.
func (c *Character) SummonDied(summon, killer attackable.Combatant) {
	c.notifyDeath(summon, killer)
}
