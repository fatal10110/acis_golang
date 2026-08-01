package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// Stance is the client-visible animation caused by a sit/stand transition.
type Stance int

const (
	StanceSitting Stance = iota
	StanceStanding
	StanceFakeDeathStart
	StanceFakeDeathStop
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

// SetStanceBroadcaster records the packet-layer hook for sit and fake-death
// animation changes.
func (c *Character) SetStanceBroadcaster(broadcast func(Stance)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastStance = broadcast
}

// SetFakeDeathReviveBroadcaster records the packet-layer hook sent after a
// fake-death stand-up animation.
func (c *Character) SetFakeDeathReviveBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastFakeDeathRevive = broadcast
}

// Sit changes to the ordinary seated stance and broadcasts it.
func (c *Character) Sit() bool {
	changed := c.SetStanding(false)
	c.broadcastStanceChange(StanceSitting)
	return changed
}

// StartFakeDeath changes to the fake-death stance and broadcasts it.
func (c *Character) StartFakeDeath() bool {
	changed := c.SetStanding(false)
	c.broadcastStanceChange(StanceFakeDeathStart)
	return changed
}

// StopFakeDeath stands up and sends the matching fake-death revive visual.
func (c *Character) StopFakeDeath() bool {
	if c.Dead() {
		return false
	}
	changed := c.SetStanding(true)
	c.broadcastStanceChange(StanceFakeDeathStop)
	c.stateMu.RLock()
	revive := c.broadcastFakeDeathRevive
	c.stateMu.RUnlock()
	if revive != nil {
		revive()
	}
	return changed
}

func (c *Character) broadcastStanceChange(stance Stance) {
	c.stateMu.RLock()
	broadcast := c.broadcastStance
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast(stance)
	}
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
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if delay <= 0 {
		delete(c.disabledItems, objectID)
		return
	}
	if c.disabledItems == nil {
		c.disabledItems = make(map[int32]time.Time)
	}
	c.disabledItems[objectID] = time.Now().Add(delay)
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

	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	until, ok := c.disabledItems[objectID]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
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

// AttachLive installs live as this character's crowd-control/movement
// runtime state. EnterWorld's live-attach and AllSkillsDisabled's read both
// go through stateMu, so a concurrent caller (e.g. a persisted-state
// assertion racing live setup) never observes a torn pointer.
func (c *Character) AttachLive(live *creature.Live) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.Live = live
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
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.recentFakeDeathUntil = time.Now().Add(recentFakeDeathGrace)
}

// RecentFakeDeath reports whether this player is still within its
// post-fake-death grace period, during which hostile NPC AI won't
// retarget it.
func (c *Character) RecentFakeDeath() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return time.Now().Before(c.recentFakeDeathUntil)
}
