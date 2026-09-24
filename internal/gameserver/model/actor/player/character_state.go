package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

func (c *Character) initStateLocked() {
	if c.stateInit {
		return
	}
	c.running = true
	c.standing = true
	c.stateInit = true
}

// Running reports whether this character is in run mode.
func (c *Character) Running() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.stateInit || c.running
}

// SetRunning updates run mode and reports whether it changed.
func (c *Character) SetRunning(running bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.running == running {
		return false
	}
	c.running = running
	return true
}

// Standing reports whether this character is standing rather than sitting.
func (c *Character) Standing() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.stateInit || c.standing
}

// SetStanding updates sit/stand mode and reports whether it changed.
func (c *Character) SetStanding(standing bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.standing == standing {
		return false
	}
	c.standing = standing
	return true
}

// Sit changes to the ordinary seated stance and broadcasts it.
func (c *Character) Sit() bool {
	changed := c.SetStanding(false)
	c.broadcastStanceChange(event.StanceSitting)
	return changed
}

// StandUp changes to the standing stance and broadcasts it.
func (c *Character) StandUp() bool {
	changed := c.SetStanding(true)
	c.broadcastStanceChange(event.StanceStanding)
	return changed
}

// StartFakeDeath changes to the fake-death stance and broadcasts it.
func (c *Character) StartFakeDeath() bool {
	changed := c.SetStanding(false)
	c.broadcastStanceChange(event.StanceFakeDeathStart)
	return changed
}

// StopFakeDeath stands up and sends the matching fake-death revive visual.
func (c *Character) StopFakeDeath() bool {
	if c.Dead() {
		return false
	}
	changed := c.SetStanding(true)
	c.broadcastStanceChange(event.StanceFakeDeathStop)
	c.emit(event.FakeDeathRevived{})
	return changed
}

func (c *Character) broadcastStanceChange(stance event.Stance) {
	c.emit(event.StanceChanged{Stance: stance})
}

// InCombat reports whether this character has started an attack stance.
func (c *Character) InCombat() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.inCombat
}

// SetInCombat updates attack stance and reports whether it changed.
func (c *Character) SetInCombat(inCombat bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.inCombat == inCombat {
		return false
	}
	c.inCombat = inCombat
	return true
}

// Flying reports whether this character is in a flying transform/mount state.
func (c *Character) Flying() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.flying
}

// SetFlying updates flying state and reports whether it changed.
func (c *Character) SetFlying(flying bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.flying == flying {
		return false
	}
	c.flying = flying
	return true
}

// Transformed reports whether this character is in a non-flying transform state.
func (c *Character) Transformed() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.transformed
}

// SetTransformed updates transform state and reports whether it changed.
func (c *Character) SetTransformed(transformed bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.transformed == transformed {
		return false
	}
	c.transformed = transformed
	return true
}

// Operating reports whether this character is operating a store/workshop.
func (c *Character) Operating() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.operating
}

// SetOperating updates store/workshop operation state and reports whether it changed.
func (c *Character) SetOperating(operating bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.operating == operating {
		return false
	}
	c.operating = operating
	return true
}

// Fishing reports whether this character is currently fishing.
func (c *Character) Fishing() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.fishing
}

// SetFishing updates fishing state and reports whether it changed.
func (c *Character) SetFishing(fishing bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.fishing == fishing {
		return false
	}
	c.fishing = fishing
	return true
}

// IsHero reports whether this character currently has hero status.
func (c *Character) IsHero() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.hero
}

// SetHero updates hero status and reports whether it changed.
func (c *Character) SetHero(hero bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.hero == hero {
		return false
	}
	c.hero = hero
	return true
}

// DisableItem marks an inventory object id unusable until delay expires.
func (c *Character) DisableItem(objectID int32, delay time.Duration) {
	if objectID <= 0 {
		return
	}
	until := c.Now().Add(delay)
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if delay <= 0 {
		delete(c.disabledItems, objectID)
		return
	}
	if c.disabledItems == nil {
		c.disabledItems = make(map[int32]time.Time)
	}
	c.disabledItems[objectID] = until
}

// ItemDisabled reports whether an inventory object id is still disabled.
// Matches Java's Playable.isItemDisabled: the AllSkillsDisabled lock only
// short-circuits every id when at least one item is already tracked as
// disabled (Playable.java:355-359) — with no disabled item at all, the lock
// has no effect here.
func (c *Character) ItemDisabled(objectID int32) bool {
	if objectID <= 0 {
		return false
	}
	c.stateMu.Lock()
	empty := len(c.disabledItems) == 0
	c.stateMu.Unlock()
	if empty {
		return false
	}
	// AllSkillsDisabled takes stateMu itself, so it must run outside the
	// lock above to avoid a self-deadlock on the non-reentrant RWMutex.
	if c.AllSkillsDisabled() {
		return true
	}

	now := c.Now()
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	until, ok := c.disabledItems[objectID]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(c.disabledItems, objectID)
	return false
}

// AllSkillsDisabled mirrors Java's Creature.isAllSkillsDisabled(): the
// crowd-control states that block skill and item use. Java also unions a raw
// Duel-defeat lock (Creature._allSkillsDisabled, set/cleared only by
// PlayerStatus/Player's Duel handling), which this port does not model since
// Duel isn't ported yet.
func (c *Character) AllSkillsDisabled() bool {
	live := c.liveLocked()
	if live == nil {
		return false
	}
	return live.Stunned() || live.ImmobileUntilAttacked() || live.Sleeping() || live.Paralyzed() || live.Afraid()
}

// Now reads the clock this character's queue runs on once Attach has
// installed Live. A character that was never attached has no queue and reads
// the wall clock, which is what a Pool queue reads too.
func (c *Character) Now() time.Time {
	if live := c.liveLocked(); live != nil {
		return live.Now()
	}
	return time.Now()
}

// liveLocked reads the Live pointer under stateMu, for call sites that
// cannot rely on Live having been set before any other goroutine can see
// this Character.
func (c *Character) liveLocked() *creature.Live {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.Live
}

// recentFakeDeathGrace is how long a player is exempt from hostile NPC
// auto-targeting after standing up from Fake Death, matching the shipped
// PlayerFakeDeathUpProtection default (players.properties).
const recentFakeDeathGrace = 5 * time.Second

// MarkRecentFakeDeath starts this player's post-fake-death grace period.
func (c *Character) MarkRecentFakeDeath() {
	until := c.Now().Add(recentFakeDeathGrace)
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.recentFakeDeathUntil = until
}

// RecentFakeDeath reports whether this player is still within its
// post-fake-death grace period, during which hostile NPC AI won't
// retarget it.
func (c *Character) RecentFakeDeath() bool {
	now := c.Now()
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return now.Before(c.recentFakeDeathUntil)
}
