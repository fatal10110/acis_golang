// Package attack owns the physical auto-attack controller shared by live
// creatures.
package attack

import (
	"sync"
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/rs/zerolog"
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

// CreatureActor is the owner state a physical attack controller reads and
// updates while starting attacks.
type CreatureActor interface {
	attackable.Combatant
	skilltarget.Actor

	AttackDisabled() bool
	MovementDisabled() bool
	InAttackRange(attackable.Combatant) bool
	Knows(attackable.Combatant) bool
	CanSee(attackable.Combatant) bool

	AttackType() item.WeaponType
	AttackSpeed() int
	PhysicalAttackRange() int
	PoleAttackAngle() int
	PoleAttackCountMax() int
	ForEachKnownCombatantInRadius(int, func(attackable.Combatant))
	WeaponReuseDelay() time.Duration
	WeaponGrade() int
	SoulshotCharged() bool
	SetChargedShot(kind item.ShotKind, charged bool)

	Position() (int, int, int)
	Heading() int
	Dead() bool
	SetHeadingTo(attackable.Combatant)
	MakeAttackHit(target attackable.Combatant, split bool) Hit
	BroadcastAttack(event.Attack) error
	ConsumeBowMP()
}

// PlayableActor is a creature controlled by a player or owned by one.
type PlayableActor interface {
	CreatureActor

	InPeaceZone() bool
	TryToIdle()
	// TestCursesOnAttack applies the raid curse for attacking a raid-related
	// target and reports whether it blocked the attack.
	TestCursesOnAttack(attackable.Combatant) bool
}

// PlayerActor is the player-only attack surface.
type PlayerActor interface {
	PlayableActor

	CheckAndEquipArrows() bool
	WeaponMPConsume() int
	MP() int
	ConsumeBowShot()
	NotifyBowDraw(gaugeMs int)
	ClearRecentFakeDeath()
	ClientActionFailed()
	// NotePvPAttack records a resolved physical hit for PvP flagging.
	NotePvPAttack(attackable.Combatant)
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
	sink           event.Sink
	log            zerolog.Logger
}

// SetQueue runs the controller's scheduled hit and finish callbacks as tasks
// on q, the owning actor's queue.
func (c *Controller) SetQueue(q *sim.Queue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.afterFunc = func(d time.Duration, fn func()) scheduledTimer { return q.After(d, fn) }
}

// SetLogger records where a panic recovered from a scheduled attack callback
// is logged. The zero value discards it.
func (c *Controller) SetLogger(log zerolog.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log = log
}

// Every constructor takes the sink that receives AttackStarted, as each
// attack animation starts (before its hits are scheduled or broadcast), and
// AttackFinished, once it finishes (the swing lands and, for non-bow weapons,
// the actor is free to attack again). A nil sink drops both.

// NewCreature returns a base creature attack controller.
func NewCreature(actor CreatureActor, sink event.Sink) *Controller {
	return &Controller{actor: actor, sink: sink}
}

// NewPlayable returns an attack controller with playable-specific rules.
func NewPlayable(actor PlayableActor, sink event.Sink) *Controller {
	return &Controller{actor: actor, playable: actor, sink: sink}
}

// NewPlayer returns an attack controller with player-specific rules.
func NewPlayer(actor PlayerActor, sink event.Sink) *Controller {
	return &Controller{actor: actor, playable: actor, player: actor, sink: sink}
}

// NewAttackable returns an attack controller with hostile NPC-specific
// rules.
func NewAttackable(actor CreatureActor, sink event.Sink) *Controller {
	return &Controller{actor: actor, attackable: true, sink: sink}
}

func (c *Controller) emit(e event.Event) {
	if c.sink != nil {
		c.sink.Emit(e)
	}
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
	if t, ok := target.(skilltarget.Actor); !ok || !t.AttackableBy(c.actor) {
		return false
	}
	if !c.actor.CanSee(target) {
		return false
	}

	if c.playable != nil {
		if target.Kind().Playable() {
			if c.playable.InPeaceZone() || target.InPeaceZone() {
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
		if target.FakeDeath() {
			return false
		}
	}

	return true
}

// DoAttack starts one physical attack animation against target. The attack
// itself is never aborted by a broadcast failure — c.start has already
// scheduled the hit landings by the time BroadcastAttack runs — but a
// non-nil return still reports that the animation packet didn't reach
// observers.
func (c *Controller) DoAttack(target attackable.Combatant) error {
	if target == nil || c.actor == nil {
		return nil
	}

	c.emit(event.AttackStarted{})

	attackTime := time.Duration(formulas.TimeBetweenAttacks(max(1, c.actor.AttackSpeed()))) * time.Millisecond
	c.actor.SetHeadingTo(target)
	attackType := c.actor.AttackType()

	var hits []Hit
	var landings []scheduledHit
	var bowReuse time.Duration
	switch attackType {
	case item.WeaponDual, item.WeaponDualFist:
		hits = []Hit{c.makeHit(target, true), c.makeHit(target, true)}
		landings = []scheduledHit{
			{hits: hits[:1], delay: attackTime / 2},
			{hits: hits[1:], delay: attackTime},
		}
	case item.WeaponBow:
		if c.player != nil {
			c.player.ConsumeBowShot()
		}
		c.actor.ConsumeBowMP()
		hits = []Hit{c.makeHit(target, false)}
		landings = []scheduledHit{{hits: hits, delay: attackTime}}
		bowReuse = c.scaledBowReuse()
	case item.WeaponPole:
		hits = []Hit{c.makeHit(target, false)}
		maxTargets := c.actor.PoleAttackCountMax()
		if maxTargets > 1 {
			x, y, z := c.actor.Position()
			origin := location.OrientedLocation{
				Location: location.Location{X: x, Y: y, Z: z},
				Heading:  c.actor.Heading(),
			}
			angle := c.actor.PoleAttackAngle()
			primaryIsPlayable := target.Kind().Playable()
			c.actor.ForEachKnownCombatantInRadius(c.actor.PhysicalAttackRange(), func(candidate attackable.Combatant) {
				if len(hits) >= maxTargets || candidate.ObjectID() == c.actor.ObjectID() || candidate.ObjectID() == target.ObjectID() {
					return
				}
				tx, ty, tz := candidate.Position()
				if !origin.IsFacing(location.Location{X: tx, Y: ty, Z: tz}, angle) {
					return
				}
				rules, ok := candidate.(skilltarget.Actor)
				if !ok || !rules.AttackableBy(c.actor) {
					return
				}
				if c.playable != nil && candidate.Kind().Playable() {
					if candidate.InPeaceZone() || !primaryIsPlayable || !rules.AttackableWithoutForceBy(c.playable) {
						return
					}
				}
				hits = append(hits, c.makeHit(candidate, false))
			})
		}
		landings = []scheduledHit{{hits: hits, delay: attackTime / 2}}
	default:
		hits = []Hit{c.makeHit(target, false)}
		landings = []scheduledHit{{hits: hits, delay: attackTime / 2}}
	}

	c.start(attackType, attackTime, landings, bowReuse)
	if attackType == item.WeaponBow && c.player != nil {
		c.player.NotifyBowDraw(int(attackTime/time.Millisecond) + int(bowReuse/time.Millisecond))
	}
	err := c.actor.BroadcastAttack(c.snapshot(hits))

	if c.player != nil {
		c.player.ClearRecentFakeDeath()
	}
	return err
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
	hits  []Hit
	delay time.Duration
}

func (c *Controller) start(weapon item.WeaponType, attackTime time.Duration, hits []scheduledHit, bowReuse time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopTimerLocked()
	c.attackSeq++
	seq := c.attackSeq
	c.attacking = true
	c.bowCooling = weapon == item.WeaponBow
	c.inHitAnimation = true

	if len(hits) > 0 {
		c.scheduleLocked(hits[0].delay+300*time.Millisecond, func() { c.clearHitAnimation(seq) })
	}

	finishAt := attackTime
	finish := func() { c.finishAttack(seq) }
	if weapon == item.WeaponBow {
		finish = func() { c.finishBow(seq, bowReuse) }
	}
	if weapon == item.WeaponDual || weapon == item.WeaponDualFist {
		finishAt = attackTime * 3 / 2
	}
	if len(hits) == 0 {
		c.scheduleLocked(finishAt, finish)
		return
	}
	c.scheduleHitLocked(seq, hits, 0, finishAt, finish)
}

func (c *Controller) scheduleHitLocked(seq uint64, groups []scheduledHit, index int, finishAt time.Duration, finish func()) {
	group := groups[index]
	delay := group.delay
	if index > 0 {
		delay -= groups[index-1].delay
	}
	c.scheduleLocked(delay, func() {
		c.deliverHits(seq, group.hits)

		c.mu.Lock()
		defer c.mu.Unlock()
		if seq != c.attackSeq {
			return
		}
		if index+1 < len(groups) {
			c.scheduleHitLocked(seq, groups, index+1, finishAt, finish)
			return
		}
		c.scheduleLocked(max(time.Duration(0), finishAt-group.delay), finish)
	})
}

func (c *Controller) snapshot(hits []Hit) event.Attack {
	x, y, z := c.actor.Position()
	s := event.Attack{
		AttackerID: c.actor.ObjectID(),
		X:          x,
		Y:          y,
		Z:          z,
		Hits:       make([]event.AttackHit, 0, len(hits)),
	}
	for _, hit := range hits {
		s.Hits = append(s.Hits, event.AttackHit{
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

func (c *Controller) deliverHits(seq uint64, hits []Hit) {
	c.mu.RLock()
	active := seq == c.attackSeq
	c.mu.RUnlock()
	if !active || len(hits) == 0 || hits[0].Target == nil || c.actor.AlikeDead() {
		return
	}
	if !c.actor.Knows(hits[0].Target) || hits[0].Target.AlikeDead() {
		c.Stop()
		return
	}
	if hits[0].Target.RaidRelated() {
		if c.playable != nil && c.playable.TestCursesOnAttack(hits[0].Target) {
			c.Stop()
			return
		}
	}
	for _, hit := range hits {
		c.deliverHit(hit)
	}
}

func (c *Controller) deliverHit(hit Hit) {
	if hit.Target == nil || !c.actor.Knows(hit.Target) || hit.Target.AlikeDead() {
		return
	}
	if !hit.Miss {
		// CreatureAttack.onHitTimer, CreatureAttack.java:134-137: a landed
		// physical hit discharges the actor's soulshot charge, independent
		// of actor type and of damage dealt, once the target-liveness guard
		// above (mirroring Java's own mainTarget.isDead() check at line 115)
		// passes.
		c.actor.SetChargedShot(item.ShotSoul, false)
	}
	if c.player != nil {
		c.player.NotePvPAttack(hit.Target)
	}
	if hit.Miss || hit.Damage <= 0 {
		return
	}
	hit.Target.TakeDamage(hit.Damage, c.actor)
}

func (c *Controller) finishBow(seq uint64, reuse time.Duration) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}

	c.attacking = false

	if reuse > 0 {
		c.scheduleLocked(reuse, func() { c.clearBowCooldown(seq) })
		c.mu.Unlock()
		return
	}

	c.bowCooling = false
	c.mu.Unlock()

	c.emit(event.AttackFinished{})
}

func (c *Controller) finishAttack(seq uint64) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}
	c.attacking = false
	c.mu.Unlock()

	c.emit(event.AttackFinished{})
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
	c.mu.Unlock()

	c.emit(event.AttackFinished{})
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
		log := c.log
		source = func(delay time.Duration, fn func()) scheduledTimer {
			return time.AfterFunc(delay, func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Msg("attack: recovered panic in scheduled callback")
					}
				}()
				fn()
			})
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
