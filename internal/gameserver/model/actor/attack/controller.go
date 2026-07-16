// Package attack owns the physical auto-attack controller shared by live
// creatures.
package attack

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

const (
	// HitSoulshot marks a hit using a soulshot charge.
	HitSoulshot = 0x10
	// HitCritical marks a critical hit.
	HitCritical = 0x20
	// HitShield marks a shield-blocked hit.
	HitShield = 0x40
	// HitMiss marks an evaded hit.
	HitMiss = 0x80
)

// SnapshotHit is one target entry in an attack animation broadcast.
type SnapshotHit struct {
	TargetID int32
	Damage   int
	Flags    uint8
}

// Snapshot is the immutable data needed to broadcast one attack.
type Snapshot struct {
	AttackerID int32
	X, Y, Z    int
	Hits       []SnapshotHit
}

// CreatureActor is the owner state a physical attack controller reads and
// updates while starting attacks.
type CreatureActor interface {
	attackable.Combatant

	AttackDisabled() bool
	MovementDisabled() bool
	InAttackRange(attackable.Combatant) bool
	Knows(attackable.Combatant) bool
	CanSee(attackable.Combatant) bool

	AttackType() item.WeaponType
	AttackSpeed() int
	WeaponReuseDelay() time.Duration
	WeaponGrade() int
	SoulshotCharged() bool

	Position() (int, int, int)
	SetHeadingTo(attackable.Combatant)
	MakeAttackHit(target attackable.Combatant, split bool) Hit
	BroadcastAttack(Snapshot)
}

// PlayableActor is a creature controlled by a player or owned by one.
type PlayableActor interface {
	CreatureActor

	InPeaceZone() bool
	TryToIdle()
}

// PlayerActor is the player-only attack surface.
type PlayerActor interface {
	PlayableActor

	CheckAndEquipArrows() bool
	WeaponMPConsume() int
	MP() int
	ClearRecentFakeDeath()
	ClientActionFailed()
}

// Hit is one precomputed physical attack result.
type Hit struct {
	Target   attackable.Combatant
	TargetID int32
	Damage   int
	Crit     bool
	Miss     bool
	Shield   formulas.ShieldDefense
}

type scheduledTimer interface {
	Stop() bool
}

type afterFunc func(time.Duration, func()) scheduledTimer

// Controller coordinates attack validation, animation state and packet
// broadcast for one creature.
//
// mu guards every mutable field below. Timers take the same lock before
// changing state.
type Controller struct {
	actor    CreatureActor
	playable PlayableActor
	player   PlayerActor

	attackable bool

	mu             sync.RWMutex
	attacking      bool
	bowCooling     bool
	inHitAnimation bool
	timers         []scheduledTimer
	attackSeq      uint64
	afterFunc      afterFunc
	finished       func()
	started        func()
}

// SetFinished records the callback invoked once an attack animation
// finishes (the swing lands and, for non-bow weapons, the actor is free to
// attack again). A nil callback (the default) makes it a no-op.
func (c *Controller) SetFinished(finished func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finished = finished
}

// SetStarted records the callback invoked as each attack animation starts,
// before its hits are scheduled or broadcast. A nil callback (the default)
// makes it a no-op.
func (c *Controller) SetStarted(started func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = started
}

// NewCreature returns a base creature attack controller.
func NewCreature(actor CreatureActor) *Controller {
	return &Controller{actor: actor}
}

// NewPlayable returns an attack controller with playable-specific rules.
func NewPlayable(actor PlayableActor) *Controller {
	return &Controller{actor: actor, playable: actor}
}

// NewPlayer returns an attack controller with player-specific rules.
func NewPlayer(actor PlayerActor) *Controller {
	return &Controller{actor: actor, playable: actor, player: actor}
}

// NewAttackable returns an attack controller with hostile NPC-specific
// rules.
func NewAttackable(actor CreatureActor) *Controller {
	return &Controller{actor: actor, attackable: true}
}

// AttackingNow reports whether an attack animation is still active.
func (c *Controller) AttackingNow() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.attacking
}

// BowCoolingDown reports whether a bow is waiting for its reuse delay.
func (c *Controller) BowCoolingDown() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bowCooling
}

// InHitAnimation reports whether the actor is still in its local hit
// animation window.
func (c *Controller) InHitAnimation() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inHitAnimation
}

// CanAttack reports whether target may be physically attacked now.
func (c *Controller) CanAttack(target attackable.Combatant) bool {
	if target == nil || c.actor == nil {
		return false
	}
	if c.actor.AttackDisabled() {
		return false
	}
	c.mu.RLock()
	busy := c.attacking || c.bowCooling
	c.mu.RUnlock()
	if busy {
		return false
	}
	if c.actor.MovementDisabled() && !c.actor.InAttackRange(target) {
		return false
	}
	if !c.actor.Knows(target) {
		return false
	}
	if t, ok := target.(interface{ AttackableBy(CreatureActor) bool }); !ok || !t.AttackableBy(c.actor) {
		return false
	}
	if !c.actor.CanSee(target) {
		return false
	}

	if c.playable != nil {
		if t, ok := target.(interface {
			Playable() bool
			InPeaceZone() bool
		}); ok && t.Playable() {
			if c.playable.InPeaceZone() || t.InPeaceZone() {
				return false
			}
		}
	}

	if c.player != nil {
		switch c.actor.AttackType() {
		case item.WeaponFishingRod:
			return false
		case item.WeaponBow:
			if !c.player.CheckAndEquipArrows() {
				return false
			}
			if mp := c.player.WeaponMPConsume(); mp > 0 && mp > c.player.MP() {
				return false
			}
		}
	}

	if c.attackable {
		if t, ok := target.(interface{ FakeDeath() bool }); ok && t.FakeDeath() {
			return false
		}
	}

	return true
}

// DoAttack starts one physical attack animation against target.
func (c *Controller) DoAttack(target attackable.Combatant) {
	if target == nil || c.actor == nil {
		return
	}

	c.mu.RLock()
	started := c.started
	c.mu.RUnlock()
	if started != nil {
		started()
	}

	attackTime := time.Duration(formulas.TimeBetweenAttacks(max(1, c.actor.AttackSpeed()))) * time.Millisecond
	c.actor.SetHeadingTo(target)
	attackType := c.actor.AttackType()

	var hits []Hit
	var landings []scheduledHit
	switch attackType {
	case item.WeaponDual, item.WeaponDualFist:
		hits = []Hit{c.makeHit(target, true), c.makeHit(target, true)}
		landings = []scheduledHit{
			{hit: hits[0], delay: attackTime / 4},
			{hit: hits[1], delay: attackTime * 3 / 4},
		}
	case item.WeaponBow:
		hits = []Hit{c.makeHit(target, false)}
		landings = []scheduledHit{{hit: hits[0], delay: attackTime}}
	case item.WeaponPole:
		hits = []Hit{c.makeHit(target, false)}
		landings = []scheduledHit{{hit: hits[0], delay: attackTime / 2}}
	default:
		hits = []Hit{c.makeHit(target, false)}
		landings = []scheduledHit{{hit: hits[0], delay: attackTime / 2}}
	}

	c.start(attackType, attackTime, landings)
	c.actor.BroadcastAttack(c.snapshot(hits))

	if c.player != nil {
		c.player.ClearRecentFakeDeath()
	}
}

// Stop aborts the current attack and clears any pending bow cooldown.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.stopTimerLocked()
	c.attackSeq++
	c.attacking = false
	c.inHitAnimation = false
	c.bowCooling = false
	c.mu.Unlock()

	if c.playable != nil {
		c.playable.TryToIdle()
	}
	if c.player != nil {
		c.player.ClientActionFailed()
	}
}

func (c *Controller) makeHit(target attackable.Combatant, split bool) Hit {
	hit := c.actor.MakeAttackHit(target, split)
	if hit.Target == nil {
		hit.Target = target
	}
	if hit.TargetID == 0 {
		hit.TargetID = target.ObjectID()
	}
	return hit
}

type scheduledHit struct {
	hit   Hit
	delay time.Duration
}

func (c *Controller) start(weapon item.WeaponType, attackTime time.Duration, hits []scheduledHit) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopTimerLocked()
	c.attackSeq++
	seq := c.attackSeq
	c.attacking = true
	c.bowCooling = weapon == item.WeaponBow
	c.inHitAnimation = true

	lastLanding := time.Duration(0)
	for _, hit := range hits {
		hit := hit
		if hit.delay > lastLanding {
			lastLanding = hit.delay
		}
		c.scheduleLocked(hit.delay, func() { c.deliverHit(seq, hit.hit) })
	}
	c.scheduleLocked(lastLanding+300*time.Millisecond, func() { c.clearHitAnimation(seq) })

	if weapon == item.WeaponBow {
		c.scheduleLocked(attackTime, func() { c.finishBow(seq) })
		return
	}
	c.scheduleLocked(attackTime, func() { c.finishAttack(seq) })
}

func (c *Controller) snapshot(hits []Hit) Snapshot {
	x, y, z := c.actor.Position()
	s := Snapshot{
		AttackerID: c.actor.ObjectID(),
		X:          x,
		Y:          y,
		Z:          z,
		Hits:       make([]SnapshotHit, 0, len(hits)),
	}
	for _, hit := range hits {
		s.Hits = append(s.Hits, SnapshotHit{
			TargetID: hit.TargetID,
			Damage:   hit.Damage,
			Flags:    c.hitFlags(hit),
		})
	}
	return s
}

func (c *Controller) hitFlags(hit Hit) uint8 {
	if hit.Miss {
		return HitMiss
	}

	var flags uint8
	if c.actor.SoulshotCharged() {
		flags |= HitSoulshot | uint8(c.actor.WeaponGrade())
	}
	if hit.Crit {
		flags |= HitCritical
	}
	if hit.Shield != formulas.ShieldFailed {
		flags |= HitShield
	}
	return flags
}

type damageReceiver interface {
	TakeDamage(int, creature.DeathActor) bool
}

func (c *Controller) deliverHit(seq uint64, hit Hit) {
	c.mu.RLock()
	active := seq == c.attackSeq
	c.mu.RUnlock()
	if !active || hit.Target == nil || hit.Miss || hit.Damage <= 0 {
		return
	}
	if c.actor.AlikeDead() || !c.actor.Knows(hit.Target) || hit.Target.AlikeDead() {
		return
	}
	target, ok := hit.Target.(damageReceiver)
	if !ok {
		return
	}
	target.TakeDamage(hit.Damage, c.actor)
}

func (c *Controller) finishBow(seq uint64) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}

	c.attacking = false

	reuse := c.scaledBowReuse()
	if reuse > 0 {
		c.scheduleLocked(reuse, func() { c.clearBowCooldown(seq) })
		c.mu.Unlock()
		return
	}

	c.bowCooling = false
	finished := c.finished
	c.mu.Unlock()

	if finished != nil {
		finished()
	}
}

func (c *Controller) finishAttack(seq uint64) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}
	c.attacking = false
	finished := c.finished
	c.mu.Unlock()

	if finished != nil {
		finished()
	}
}

func (c *Controller) clearHitAnimation(seq uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if seq == c.attackSeq {
		c.inHitAnimation = false
	}
}

func (c *Controller) clearBowCooldown(seq uint64) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}
	c.bowCooling = false
	finished := c.finished
	c.mu.Unlock()

	if finished != nil {
		finished()
	}
}

func (c *Controller) scaledBowReuse() time.Duration {
	reuse := c.actor.WeaponReuseDelay()
	if reuse <= 0 {
		return 0
	}
	return time.Duration(int64(reuse) * 345 / int64(max(1, c.actor.AttackSpeed())))
}

func (c *Controller) scheduleLocked(delay time.Duration, f func()) {
	source := c.afterFunc
	if source == nil {
		source = func(delay time.Duration, f func()) scheduledTimer {
			return time.AfterFunc(delay, f)
		}
	}
	c.timers = append(c.timers, source(delay, f))
}

func (c *Controller) stopTimerLocked() {
	for _, timer := range c.timers {
		timer.Stop()
	}
	c.timers = nil
}
