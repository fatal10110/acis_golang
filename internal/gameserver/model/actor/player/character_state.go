package player

import (
	"math"
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
	c.initStateLocked()
	if c.running == running {
		c.stateMu.Unlock()
		return false
	}
	c.running = running
	c.stateMu.Unlock()
	c.refreshMoveSpeed()
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

// sitStandDelay is how long a sit-down or a stand-up holds the character
// before it takes control back.
const sitStandDelay = 2500 * time.Millisecond

// Fake death lies down over fakeDeathSitMillis and gets back up over
// fakeDeathStandMillis, each divided by the movement speed multiplier.
const (
	fakeDeathSitMillis   = 3000
	fakeDeathStandMillis = 2500
)

// ChangePosture sits the character down or stands it up, starting the
// sit/stand transition, and reports whether the posture changed. An
// unchanged posture starts no transition.
func (c *Character) ChangePosture(standing bool) bool {
	return c.changePosture(standing, false)
}

// Sit changes to the ordinary seated stance, starts the sit-down transition
// and broadcasts it.
func (c *Character) Sit() bool {
	changed := c.changePosture(false, true)
	c.broadcastStanceChange(event.StanceSitting)
	return changed
}

// StandUp changes to the standing stance, starts the stand-up transition
// and broadcasts it.
func (c *Character) StandUp() bool {
	changed := c.changePosture(true, true)
	c.broadcastStanceChange(event.StanceStanding)
	return changed
}

// changePosture sets the posture and, when it changed or always is set,
// starts the matching transition, in one stateMu section: a posture change
// on another goroutine cannot land between the two and leave the posture
// and the transition disagreeing. It reports whether the posture changed.
func (c *Character) changePosture(standing, always bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	changed := c.standing != standing
	c.standing = standing
	if changed || always {
		c.beginPostureTransitionLocked(standing, sitStandDelay, false)
	}
	return changed
}

// Seated reports whether the character sits and has finished sitting down.
func (c *Character) Seated() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.stateInit && !c.standing && !c.sittingNow
}

// SittingNow reports whether the character is still sitting down: the
// seated posture has been taken but the sit-down transition has not ended.
func (c *Character) SittingNow() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.sittingNow
}

// StandingNow reports whether the character is still standing up: the
// standing posture has been taken but the stand-up transition has not ended.
func (c *Character) StandingNow() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.standingNow
}

// beginPostureTransitionLocked starts a sit-down (standing false) or
// stand-up transition, replacing any transition still running. It ends
// after delay with a PostureSettled event; endsFakeDeath makes its end also
// end fake death. A character without a live runtime has no queue to end it
// on and starts none, so it reports false and the caller settles at once.
// c.stateMu must be held.
func (c *Character) beginPostureTransitionLocked(standing bool, delay time.Duration, endsFakeDeath bool) bool {
	if c.Live == nil {
		return false
	}
	c.postureGen++
	gen := c.postureGen
	c.sittingNow, c.standingNow = !standing, standing
	c.afterLocked(delay, func() { c.settlePosture(gen, endsFakeDeath) })
	return true
}

// settlePosture ends the transition gen started, unless a later transition
// replaced it.
func (c *Character) settlePosture(gen uint64, endsFakeDeath bool) {
	c.stateMu.Lock()
	if gen != c.postureGen {
		c.stateMu.Unlock()
		return
	}
	c.sittingNow, c.standingNow = false, false
	if endsFakeDeath {
		c.fakeDeath = false
	}
	c.stateMu.Unlock()
	c.emit(event.PostureSettled{})
}

// FakeDead reports whether the character plays dead: from the start of
// fake death until its stand-up ends, which outlasts the effect itself.
func (c *Character) FakeDead() bool {
	c.stateMu.RLock()
	live, faking := c.Live, c.fakeDeath
	c.stateMu.RUnlock()
	return faking || live.FakeDead()
}

// StartFakeDeath lies down into fake death, starts the lie-down transition
// and broadcasts the stance.
func (c *Character) StartFakeDeath() bool {
	delay := fakeDeathDelay(fakeDeathSitMillis, c.MovementSpeedMultiplier())
	c.stateMu.Lock()
	c.initStateLocked()
	changed := c.standing
	c.standing = false
	c.fakeDeath = true
	c.beginPostureTransitionLocked(false, delay, false)
	c.stateMu.Unlock()
	c.broadcastStanceChange(event.StanceFakeDeathStart)
	return changed
}

// StopFakeDeath gets up out of fake death and sends the matching revive
// visual. The character plays dead until the stand-up ends. A dead
// character only leaves fake death.
func (c *Character) StopFakeDeath() bool {
	if c.Dead() {
		c.stateMu.Lock()
		c.fakeDeath = false
		c.stateMu.Unlock()
		return false
	}
	delay := fakeDeathDelay(fakeDeathStandMillis, c.MovementSpeedMultiplier())
	c.stateMu.Lock()
	c.initStateLocked()
	changed := !c.standing
	c.standing = true
	if !c.beginPostureTransitionLocked(true, delay, true) {
		c.fakeDeath = false
	}
	c.stateMu.Unlock()
	c.broadcastStanceChange(event.StanceFakeDeathStop)
	c.emit(event.FakeDeathRevived{})
	return changed
}

// RepeatFakeDeathStop answers another get-up request made while the
// character is already getting up out of fake death: it restarts the
// recent-fake-death grace and sends the get-up and revive visuals again. The
// running get-up is left alone and still ends fake death on time. It
// reports false and does nothing outside such a get-up, or on a dead
// character.
func (c *Character) RepeatFakeDeathStop() bool {
	if c.Dead() {
		return false
	}
	c.stateMu.RLock()
	gettingUp := c.fakeDeath && c.standingNow
	c.stateMu.RUnlock()
	if !gettingUp {
		return false
	}
	c.MarkRecentFakeDeath()
	c.broadcastStanceChange(event.StanceFakeDeathStop)
	c.emit(event.FakeDeathRevived{})
	return true
}

// fakeDeathDelay is millis divided by the movement speed multiplier,
// truncated to whole milliseconds. A zero multiplier (a player too heavy to
// move) saturates at the largest int32 millisecond count.
func fakeDeathDelay(millis float32, mult float32) time.Duration {
	ms := millis / mult
	if !(ms < math.MaxInt32) {
		return math.MaxInt32 * time.Millisecond
	}
	return time.Duration(int32(ms)) * time.Millisecond
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
	// ponytail: wall-clock default serves only never-attached fixtures; #2488 drops it.
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
