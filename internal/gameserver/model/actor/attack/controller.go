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
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
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

// hitAnimationTail is how long the hit animation lasts after the first hit
// group lands.
const hitAnimationTail = 300 * time.Millisecond

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
	BroadcastAttack(event.Attack)
	ConsumeBowMP()

	// CalcStat reads the actor's stat s over base; a landed hit reads its
	// damage absorption from it.
	CalcStat(s stat.Stat, base float64) float64
	// AddHP restores HP, clamped to max HP, and returns the applied amount.
	AddHP(amount float64) float64
}

// PlayableActor is a creature controlled by a player or owned by one.
type PlayableActor interface {
	CreatureActor

	InPeaceZone() bool
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
	// BroadcastStatus reports a change to the player's HP, MP or CP that its
	// own writer left unreported.
	BroadcastStatus()
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
// mu guards every mutable field below. Timers run on the owner's queue, but
// another actor's effect aborts the attack synchronously from its own queue
// (AbortAll from a stun → ActionsStopRequested → Stop).
type Controller struct {
	actor    CreatureActor
	playable PlayableActor
	player   PlayerActor

	attackable bool

	mu             sync.RWMutex
	attacking      bool
	bowCooling     bool
	inHitAnimation bool
	timers         []*sim.Timer
	attackSeq      uint64
	queue          *sim.Queue
	sink           event.Sink
}

// SetQueue runs the controller's scheduled hit and finish callbacks as tasks
// on q, the owning actor's queue. An attack needs one.
func (c *Controller) SetQueue(q *sim.Queue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = q
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

// DoAttack starts one physical attack animation against target. The hit
// landings are scheduled by c.start before the animation is broadcast, so
// there is nothing left that can fail once the swing is accepted.
func (c *Controller) DoAttack(target attackable.Combatant) {
	if target == nil || c.actor == nil {
		return
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
		landed := c.deliverHits(seq, group.hits)

		c.mu.Lock()
		defer c.mu.Unlock()
		if landed && index == 0 {
			// The hit animation ends on its own timer, which a later stop or
			// a new swing does not cancel.
			c.queue.After(hitAnimationTail, c.endHitAnimation)
		}
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

// deliverHits lands one hit group and reports whether it got past the
// main-target checks, whether or not the hits themselves connect.
func (c *Controller) deliverHits(seq uint64, hits []Hit) bool {
	c.mu.RLock()
	active := seq == c.attackSeq
	c.mu.RUnlock()
	if !active || len(hits) == 0 || hits[0].Target == nil || c.actor.AlikeDead() {
		return false
	}
	if !c.actor.Knows(hits[0].Target) || hits[0].Target.AlikeDead() {
		c.Stop()
		return false
	}
	if hits[0].Target.RaidRelated() {
		if c.playable != nil && c.playable.TestCursesOnAttack(hits[0].Target) {
			c.Stop()
			return false
		}
	}
	for _, hit := range hits {
		c.deliverHit(hit)
	}
	return true
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
	// The target hears of a miss before the attacker's feedback goes out.
	target, reacts := hit.Target.(attackedTarget)
	if hit.Miss && reacts {
		target.NotifyEvaded(c.actor)
	}
	// The attacker's feedback goes out before the target takes the damage.
	c.reportHit(hit)
	if hit.Miss || hit.Damage <= 0 {
		return
	}
	c.emit(event.AttackStanceRequested{})
	if reacts {
		target.NotifyAttacked(c.actor)
	}
	reflected := c.reflectedDamage(hit)
	hit.Target.TakeDamage(hit.Damage, c.actor)
	if reflected > 0 {
		c.actor.TakeDamage(reflected, hit.Target)
	}
	c.absorbDamage(hit)
	breakTargetCast(hit)
	c.emit(event.HitLanded{Target: hit.Target, Crit: hit.Crit, Reflected: reflected > 0})
}

// reflectingTarget is the target state a landed hit reads to size the
// damage the target reflects back on its attacker.
type reflectingTarget interface {
	Invul() bool
	CalcStat(s stat.Stat, base float64) float64
	MaxHPValue() float64
}

// reflectedDamage returns the share of hit's damage its target reflects
// back on the attacker, capped at the target's max HP, read before the
// target takes the hit. A bow hit and a hit on an invulnerable target
// reflect nothing, and neither does a raid-related target hit on behalf of
// a player more than 8 levels above it.
func (c *Controller) reflectedDamage(hit Hit) int {
	if c.actor.AttackType() == item.WeaponBow {
		return 0
	}
	target, ok := hit.Target.(reflectingTarget)
	if !ok || target.Invul() {
		return 0
	}
	if hit.Target.RaidRelated() {
		if level, ok := c.actingPlayerLevel(); ok && level > hit.Target.Level()+8 {
			return 0
		}
	}
	percent := target.CalcStat(stat.ReflectDamagePercent, 0)
	if percent <= 0 {
		return 0
	}
	return min(int(percent/100*float64(hit.Damage)), int(target.MaxHPValue()))
}

// actingPlayerLevel returns the level of the player the actor attacks for:
// the player itself, or a summon's owner. An NPC attacks for no player.
func (c *Controller) actingPlayerLevel() (int, bool) {
	if c.player != nil {
		return c.actor.Level(), true
	}
	if c.playable == nil {
		return 0, false
	}
	owner, ok := c.actor.Owner()
	if !ok || owner == nil {
		return 0, false
	}
	return owner.Level(), true
}

// absorbDamage heals the actor by its absorbed share of hit's damage; a bow
// hit absorbs nothing. A player reports its own HP change here; NPCs and
// summons report theirs from AddHP.
func (c *Controller) absorbDamage(hit Hit) {
	if c.actor.AttackType() == item.WeaponBow {
		return
	}
	percent := c.actor.CalcStat(stat.AbsorbDamagePercent, 0)
	if percent <= 0 {
		return
	}
	if c.actor.AddHP(percent/100*float64(hit.Damage)) > 0 && c.player != nil {
		c.player.BroadcastStatus()
	}
}

// castBreakTarget is a hit target whose cast a landed hit may break.
type castBreakTarget interface {
	BreakCastOnDamage(damage float64)
}

// breakTargetCast rolls whether hit breaks its target's cast, once the
// damage, the reflected damage and the absorbed HP have all applied. A
// target the hit killed, a raid-related target and an invulnerable one
// roll nothing.
func breakTargetCast(hit Hit) {
	target, ok := hit.Target.(castBreakTarget)
	if !ok || hit.Target.AlikeDead() || hit.Target.RaidRelated() {
		return
	}
	if t, ok := hit.Target.(invulTarget); ok && t.Invul() {
		return
	}
	target.BreakCastOnDamage(float64(hit.Damage))
}

// attackedTarget is a hit target whose AI reacts to the hit: players and
// summons. A hostile NPC's aggression runs from its damage path; its attack
// stance is not modeled yet (#2730).
type attackedTarget interface {
	// NotifyAttacked reports a damaging physical hit, or an offensive
	// skill, from attacker reaching the target.
	NotifyAttacked(attacker attackable.Combatant)
	// NotifyEvaded reports a physical hit from attacker that missed the
	// target.
	NotifyEvaded(attacker attackable.Combatant)
}

// invulTarget is the target state the attacker's damage feedback reads when
// the hit resolves.
type invulTarget interface {
	Invul() bool
	Paralyzed() bool
}

// reportHit emits the attacker's damage feedback for hit. NPCs report
// nothing. A summon stays silent on a miss and against its own owner, who
// would otherwise read the hit twice.
func (c *Controller) reportHit(hit Hit) {
	if c.playable == nil {
		return
	}
	if c.player == nil {
		owner, ok := c.actor.Owner()
		if hit.Miss || !ok || owner == nil || owner.ObjectID() == hit.Target.ObjectID() {
			return
		}
	}
	e := event.HitDealt{Damage: hit.Damage, Crit: hit.Crit, Miss: hit.Miss}
	if t, ok := hit.Target.(invulTarget); ok && t.Invul() {
		e.Blocked = true
		e.Petrified = t.Paralyzed()
	}
	c.emit(e)
}

func (c *Controller) finishBow(seq uint64, reuse time.Duration) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}

	// A finished shot closes the hit animation before the AI re-runs.
	c.attacking = false
	c.inHitAnimation = false

	if reuse > 0 {
		c.scheduleLocked(reuse, func() { c.clearBowCooldown(seq) })
		c.mu.Unlock()
		c.emitRethink()
		if c.player != nil {
			c.emit(event.BowShotFinished{})
		}
		return
	}

	c.bowCooling = false
	c.mu.Unlock()

	c.emitRethink()
	c.emit(event.AttackFinished{BowReuse: true})
}

func (c *Controller) finishAttack(seq uint64) {
	c.mu.Lock()
	if seq != c.attackSeq {
		c.mu.Unlock()
		return
	}
	// A finished swing closes the hit animation before the AI re-runs.
	c.attacking = false
	c.inHitAnimation = false
	c.mu.Unlock()

	c.emit(event.AttackFinished{})
}

// endHitAnimation closes the hit animation window 300ms after the first hit
// group lands, unless the swing finished or stopped first. It is not tied to
// one swing: a swing started inside the window has its flag cleared with it.
func (c *Controller) endHitAnimation() {
	c.mu.Lock()
	c.inHitAnimation = false
	c.mu.Unlock()
	c.emitRethink()
}

// emitRethink asks an NPC's AI to re-run desire selection. Other actors'
// AI does not re-run on attack phases.
func (c *Controller) emitRethink() {
	if c.attackable {
		c.emit(event.AttackRethink{})
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

	c.emit(event.AttackFinished{BowReuse: true})
}

func (c *Controller) scaledBowReuse() time.Duration {
	reuse := c.actor.WeaponReuseDelay()
	if reuse <= 0 {
		return 0
	}
	return time.Duration(int64(reuse) * 345 / int64(max(1, c.actor.AttackSpeed())))
}

func (c *Controller) scheduleLocked(delay time.Duration, f func()) {
	c.timers = append(c.timers, c.queue.After(delay, f))
}

func (c *Controller) stopTimerLocked() {
	for _, timer := range c.timers {
		timer.Stop()
	}
	c.timers = nil
}
