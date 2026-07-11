// Package attack owns the physical auto-attack controller shared by live
// creatures.
package attack

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

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
	BroadcastAttack(serverpackets.AttackSnapshot)
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
	timer          *time.Timer
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

	attackTime := time.Duration(formulas.TimeBetweenAttacks(max(1, c.actor.AttackSpeed()))) * time.Millisecond
	c.actor.SetHeadingTo(target)

	var hits []Hit
	switch c.actor.AttackType() {
	case item.WeaponDual, item.WeaponDualFist:
		hits = []Hit{c.makeHit(target, true), c.makeHit(target, true)}
		c.start(c.actor.AttackType(), attackTime/2)
	case item.WeaponBow:
		hits = []Hit{c.makeHit(target, false)}
		c.start(item.WeaponBow, attackTime)
	case item.WeaponPole:
		hits = []Hit{c.makeHit(target, false)}
		c.start(item.WeaponPole, attackTime)
	default:
		hits = []Hit{c.makeHit(target, false)}
		c.start(c.actor.AttackType(), attackTime)
	}

	c.actor.BroadcastAttack(c.snapshot(hits))

	if c.player != nil {
		c.player.ClearRecentFakeDeath()
	}
}

// Stop aborts the current attack and clears any pending bow cooldown.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.stopTimerLocked()
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

func (c *Controller) start(weapon item.WeaponType, delay time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopTimerLocked()
	c.attacking = true
	c.bowCooling = weapon == item.WeaponBow
	c.inHitAnimation = true

	c.timer = time.AfterFunc(delay, func() {
		if weapon == item.WeaponBow {
			c.finishBow()
			return
		}
		c.finishAttack()
	})
}

func (c *Controller) snapshot(hits []Hit) serverpackets.AttackSnapshot {
	x, y, z := c.actor.Position()
	s := serverpackets.AttackSnapshot{
		AttackerID: c.actor.ObjectID(),
		X:          x,
		Y:          y,
		Z:          z,
		Hits:       make([]serverpackets.AttackHit, 0, len(hits)),
	}
	for _, hit := range hits {
		s.Hits = append(s.Hits, serverpackets.AttackHit{
			TargetID: hit.TargetID,
			Damage:   hit.Damage,
			Flags:    c.hitFlags(hit),
		})
	}
	return s
}

func (c *Controller) hitFlags(hit Hit) uint8 {
	if hit.Miss {
		return serverpackets.AttackHitMiss
	}

	var flags uint8
	if c.actor.SoulshotCharged() {
		flags |= serverpackets.AttackHitSoulshot | uint8(c.actor.WeaponGrade())
	}
	if hit.Crit {
		flags |= serverpackets.AttackHitCritical
	}
	if hit.Shield != formulas.ShieldFailed {
		flags |= serverpackets.AttackHitShield
	}
	return flags
}

func (c *Controller) finishBow() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.attacking = false
	c.inHitAnimation = false

	reuse := c.actor.WeaponReuseDelay()
	if reuse <= 0 {
		c.bowCooling = false
		c.timer = nil
		return
	}

	c.timer = time.AfterFunc(reuse, func() {
		c.mu.Lock()
		c.bowCooling = false
		c.timer = nil
		c.mu.Unlock()
	})
}

func (c *Controller) finishAttack() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attacking = false
	c.inHitAnimation = false
	c.timer = nil
}

func (c *Controller) stopTimerLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}
