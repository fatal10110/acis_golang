package ai

import (
	"errors"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const attackHateDecay = 6.6
const castDesireDecay = 66000
const nothingDesireDecay = 0.5

// attackDesireRange is the 3D distance past which a queued ATTACK desire
// is dropped when the actor can still choose a new intention.
const attackDesireRange = 1500

// staleThreatAge is NpcAI.java's out-of-territory stale-hate threshold
// (NpcAI.java:319): a threat entry whose last damage is at least this old
// gets its hate stopped and its queued attack desire dropped while the
// owner is out of territory.
const staleThreatAge = 90 * time.Second

// staleThreatSweepTicks approximates NpcAI.java's 10-second out-of-territory
// sweep period (NpcAI.java:311-325) in AITick (task.AITick, 1 second)
// units, since Tick already runs once per AITick rather than on its own
// fixed-rate timer.
const staleThreatSweepTicks = 10

// AttackableActor is the actor state used by the hostile NPC intention loop.
type AttackableActor interface {
	attackable.Combatant
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	ReturnHome() bool
	IsMoving() bool
	InTerritory() bool
	// SetHeadingTo faces the actor toward target, used before committing to
	// a skill cast whose animation is long enough to plant first.
	SetHeadingTo(attackable.Combatant)
	// BroadcastMoveToPawn sends a rotation-only MoveToPawn notice toward
	// target, used when a final cast attempt is rejected after movement so
	// observers still see the actor face its target.
	BroadcastMoveToPawn(target attackable.Combatant) error
}

// MoveController controls movement requests emitted by the AI loop.
type MoveController interface {
	MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error)
	MoveHome(location.Location) error
	Stop() error
}

// AttackController controls attack requests emitted by the AI loop.
type AttackController interface {
	BowCoolingDown() bool
	AttackingNow() bool
	CanAttack(attackable.Combatant) bool
	DoAttack(attackable.Combatant) error
}

// CastController controls skill-cast requests emitted by the AI loop,
// mirroring AttackController's role for AI-initiated skill casts. A nil
// CastController on an Attackable makes IntentionCast a no-op, matching an
// actor with no skills to cast.
type CastController interface {
	// Disabled reports whether the actor cannot attempt a cast at all right
	// now: already mid-cast, or every skill disabled.
	Disabled() bool
	// CastingNow reports whether a cast is currently in flight. It is kept
	// separate from Disabled because skill-disable effects do not delay attack
	// or follow intentions.
	CastingNow() bool
	// Range returns ref's cast range, used to decide whether the actor must
	// close distance on target before attempting the cast.
	Range(ref skill.Ref) int
	// CanAttempt validates the lightweight pre-movement cast gate (reuse
	// cooldown) for ref against target.
	CanAttempt(target attackable.Combatant, ref skill.Ref) bool
	// StopsMovement reports whether ref's cast animation is long enough that
	// the actor should stop moving and face target before the final cast
	// attempt.
	StopsMovement(ref skill.Ref) bool
	// SkillType returns ref's raw skillType tag, used to grant SUMMON_FRIEND
	// casts a target-lost bypass matching the reference's rotation-target
	// exemption.
	SkillType(ref skill.Ref) string
	// CanCast validates the final HP/MP/mute/reuse/item gates, immediately
	// before the cast commits.
	CanCast(target attackable.Combatant, ref skill.Ref) bool
	// MeetsHPMPDisabled reports whether the actor currently has the HP/MP
	// and is not muted for ref against target. Queued CAST desires that
	// fail this check are dropped before promotion, separately from CanCast.
	MeetsHPMPDisabled(target attackable.Combatant, ref skill.Ref) bool
	// Cast starts the cast against target. Delayed scheduling and effect
	// application are the implementation's responsibility.
	Cast(target attackable.Combatant, ref skill.Ref)
}

type intention struct {
	kind   Intention
	target attackable.Combatant
	skill  skill.Ref
}

// Attackable drives one hostile NPC's combat and wander intentions.
//
// One AI loop owns the current and next intentions. Threat and hate tables,
// and the attack desire queue, are internally synchronized so combat code
// can raise hate while the loop reads target selection. mu guards
// current/next/step: Think and Tick run on the periodic AI task's goroutine,
// but movement-arrived and attack-finished hooks can also call Think from a
// timer goroutine, and the first attack desire against an actor with no
// most-hated target calls Think from the combat path so the reaction does
// not wait for the next tick. Entry points must serialize against each other.
type Attackable struct {
	actor   AttackableActor
	move    MoveController
	attack  AttackController
	cast    CastController
	threats *attackable.ThreatTable
	hates   *attackable.HateTable
	desires *DesireQueue

	mu      sync.Mutex
	current intention
	next    intention
	step    int
	ootStep int
	// lastKind is the intention kind Think was running before the latest
	// promote, used so escort follow can tell a fresh FOLLOW from a
	// continuing one.
	lastKind Intention
	// followPulse counts thinkFollow entries so escort movement runs on
	// odd pulses (about once every two seconds at the 1s AI tick).
	followPulse int

	// now returns the current time; tickOutOfTerritory reads it instead of
	// calling time.Now directly so tests can simulate staleThreatAge
	// elapsing without a real 90-second wait.
	now func() time.Time
}

// NewAttackable builds an idle hostile NPC AI loop.
func NewAttackable(actor AttackableActor, move MoveController, attack AttackController) *Attackable {
	return &Attackable{
		actor:   actor,
		move:    move,
		attack:  attack,
		threats: attackable.NewThreatTable(actor),
		hates:   attackable.NewHateTable(actor),
		desires: NewDesireQueue(),
		current: intention{kind: IntentionIdle},
		now:     time.Now,
	}
}

// MaybeStartOffensiveFollow starts or maintains an offensive follow task
// toward target at this actor's own physical attack range. Exposed for
// AutoAttackTargetValid's queued-desire follow gate (Npc.java:2107-2110),
// called outside the AI loop's own Think step; it must not take a.mu, since
// a caller such as RandomizeHate already holds it while evaluating
// candidates.
func (a *Attackable) MaybeStartOffensiveFollow(target attackable.Combatant) (bool, error) {
	return a.move.MaybeStartOffensiveFollow(target, a.actor.PhysicalAttackRange())
}

// ObjectID returns the actor id controlled by this AI loop.
func (a *Attackable) ObjectID() int32 {
	return a.actor.ObjectID()
}

// SetCastController wires the AI loop's IntentionCast handling to
// controller. Left unset (the default), IntentionCast desires are ignored,
// matching an actor with no skills to cast.
func (a *Attackable) SetCastController(controller CastController) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cast = controller
}

// Threats returns the physical-attack threat table.
func (a *Attackable) Threats() *attackable.ThreatTable {
	return a.threats
}

// Hates returns the skill-cast hate table.
func (a *Attackable) Hates() *attackable.HateTable {
	return a.hates
}

// Desires returns the queue of weighted candidate intentions. Attack threat
// populates it automatically; a Cast desire is queued by whatever decides
// this actor should cast a skill (e.g. a monster AI script), and Think
// promotes whichever queued desire currently outweighs the rest.
func (a *Attackable) Desires() *DesireQueue {
	return a.desires
}

// AddDamageHate records an attacker in the physical threat table.
func (a *Attackable) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	a.threats.AddDamage(attacker, damage, hate)
}

// combatAttackDesireWeight is the flat ATTACK desire weight queued for raw
// combat damage, matching DefaultNpc.tryToAttack's scripted 200
// (Npc.reduceCurrentHp never routes through addAttackDesire in the
// reference; nothing there derives desire weight from damage dealt).
const combatAttackDesireWeight = 200

// AddCombatDamageHate records attacker's combat damage in the physical
// threat table — accumulating hate there to drive target selection among
// multiple attackers, unchanged from AddDamageHate — but queues its attack
// Desire at a flat weight instead of scaling it with the damage dealt.
// When the threat table had no most-hated attacker, the AI loop runs
// immediately so the first reaction does not wait for the next tick.
func (a *Attackable) AddCombatDamageHate(attacker attackable.Combatant, damage float64) {
	_, hadMostHated := a.threats.MostHated()
	a.threats.AddDamage(attacker, damage, damage)
	if attacker == nil || (a.actor.SiegeGuard() && attacker.SiegeGuard()) {
		return
	}
	a.addAttackDesire(attacker, combatAttackDesireWeight)
	a.thinkIfNoMostHated(hadMostHated, attacker)
}

// AddAttackDesire queues an attack intention that closes on the target.
// When the threat table has no most-hated attacker, the AI loop runs
// immediately so the first reaction does not wait for the next tick.
func (a *Attackable) AddAttackDesire(attacker attackable.Combatant, hate float64) {
	a.queueAttackDesire(attacker, hate, true)
}

// AddAttackDesireHold queues an attack intention that stays in place instead
// of closing on the target. Same first-reaction Think as AddAttackDesire.
func (a *Attackable) AddAttackDesireHold(attacker attackable.Combatant, hate float64) {
	a.queueAttackDesire(attacker, hate, false)
}

func (a *Attackable) queueAttackDesire(attacker attackable.Combatant, hate float64, moveToTarget bool) {
	if attacker == nil || (a.actor.SiegeGuard() && attacker.SiegeGuard()) {
		return
	}
	_, hadMostHated := a.threats.MostHated()
	a.addAttackDesireWithMove(attacker, hate, moveToTarget)
	a.thinkIfNoMostHated(hadMostHated, attacker)
}

func (a *Attackable) addAttackDesire(attacker attackable.Combatant, hate float64) {
	a.addAttackDesireWithMove(attacker, hate, true)
}

func (a *Attackable) addAttackDesireWithMove(attacker attackable.Combatant, hate float64, moveToTarget bool) {
	a.desires.AddOrUpdate(&Desire{
		Kind:         IntentionAttack,
		FinalTarget:  attacker,
		Weight:       hate,
		QueuedAt:     time.Now(),
		MoveToTarget: moveToTarget,
	})
}

const escortFollowWeight = 5

// idleFollower is implemented by an NPC that should escort a master when it
// has nothing else to do.
type idleFollower interface {
	IdleFollowTarget() attackable.Combatant
}

// followThinker is implemented by an NPC that runs escort (or loose)
// follow movement while IntentionFollow is current.
type followThinker interface {
	ThinkFollow(target attackable.Combatant, lastWasFollow bool) (clearDesire bool)
}

func (a *Attackable) addFollowDesire(target attackable.Combatant, weight float64) {
	if target == nil {
		return
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:        IntentionFollow,
		FinalTarget: target,
		Weight:      weight,
		QueuedAt:    time.Now(),
	})
}

func (a *Attackable) queueIdleFollow() {
	follower, ok := a.actor.(idleFollower)
	if !ok {
		return
	}
	a.addFollowDesire(follower.IdleFollowTarget(), escortFollowWeight)
}

func (a *Attackable) thinkFollow() error {
	if a.followPulse%2 == 0 {
		a.followPulse++
		return nil
	}
	a.followPulse++
	follower, ok := a.actor.(followThinker)
	if !ok {
		return nil
	}
	if follower.ThinkFollow(a.current.target, a.lastKind == IntentionFollow) {
		a.desires.Remove(IntentionFollow, a.current.target)
		a.current = intention{kind: IntentionIdle}
	}
	return nil
}

// thinkIfNoMostHated runs the AI loop immediately for a first attack
// desire. Think drops unknown attackers, so an unseen target keeps its
// queued desire for a later tick instead of being wiped here.
func (a *Attackable) thinkIfNoMostHated(hadMostHated bool, attacker attackable.Combatant) {
	if hadMostHated || attacker == nil || !a.actor.Knows(attacker) {
		return
	}
	_ = a.Think()
}

// RandomizeHate ports the AI side of AggroList.randomizeAttack(), driving
// EffectRandomizeHate: swaps a random valid attacker into the most-hated
// slot ahead of the current target (see ThreatTable.RandomizeAttack), then
// clears and rebuilds the queued attack desires from every threat entry so
// they match the post-swap hate table, mirroring the reference's
// updateAggro=false requeue. Reports whether a swap happened.
func (a *Attackable) RandomizeHate(valid func(attackable.Combatant) bool, pick func(int) int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.threats.RandomizeAttack(valid, pick) {
		return false
	}

	a.desires.RemoveKind(IntentionAttack)
	for _, t := range a.threats.Snapshot() {
		a.addAttackDesire(t.Attacker, t.Hate)
	}
	return true
}

// ReconsiderTarget ports the AI side of AggroList.reconsiderTarget(range),
// used when this actor can no longer act on its current target (e.g. an
// immobilize state): swaps in a replacement from the threat table (see
// ThreatTable.ReconsiderTarget) and drops the previous most-hated attacker's
// queued attack desire if one existed. It never queues a desire for the
// chosen target — AggroList.java:169-177 only calls stopHate/addDamageHate(0)
// on the list, never touches the caller's desire queue; that is left to a
// future caller, matching randomizeAttack's sibling behavior of leaving
// target acquisition outside reconsiderTarget's scope. Reports the new
// target and whether a swap happened.
func (a *Attackable) ReconsiderTarget(inRange func(attackable.Combatant) bool, valid func(attackable.Combatant) bool) (attackable.Combatant, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	prev, chosen, ok := a.threats.ReconsiderTarget(inRange, valid)
	if !ok {
		return nil, false
	}

	if prev != nil {
		a.desires.Remove(IntentionAttack, prev)
	}
	return chosen, true
}

// AddHate records an attacker in the skill-cast hate table.
func (a *Attackable) AddHate(attacker attackable.Combatant, hate float64) {
	a.hates.Add(attacker, hate)
}

// AddDefaultHate records the default skill-cast hate for an attacker this
// actor has noticed.
func (a *Attackable) AddDefaultHate(attacker attackable.Combatant) {
	a.hates.AddDefault(attacker, a.actor.InTerritory())
}

// SetWander makes the next Think process wander/return-home behavior.
func (a *Attackable) SetWander() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.current = intention{kind: IntentionWander}
}

// SetBackToPeace clears combat memory and cancels the current action. If the
// actor is outside its spawn territory, the next Think runs the return-home
// path instead of leaving it idle off leash.
func (a *Attackable) SetBackToPeace() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setBackToPeaceLocked()
}

// StopAggroHate mirrors AggroList.stopHate: zeroes target's threat hate,
// drops its queued attack desire, and returns to peace when neither hate
// table still has a most-hated entry.
func (a *Attackable) StopAggroHate(target attackable.Combatant) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopAggroHateLocked(target)
}

// ReduceAllAggroHate mirrors AggroList.reduceAllHate: subtracts amount from
// every threat entry and returns to peace when neither hate table still has
// a most-hated entry.
func (a *Attackable) ReduceAllAggroHate(amount float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reduceAllAggroHateLocked(amount)
}

func (a *Attackable) setBackToPeaceLocked() {
	a.threats.Clear()
	a.hates.Clear()
	a.desires.Clear()
	a.next = intention{}
	a.current = intention{kind: IntentionIdle}
	if !a.actor.InTerritory() {
		a.current = intention{kind: IntentionWander}
	}
	a.move.Stop()
}

func (a *Attackable) maybeBackToPeaceLocked() {
	if _, ok := a.threats.MostHated(); ok {
		return
	}
	if _, ok := a.hates.MostHated(); ok {
		return
	}
	a.setBackToPeaceLocked()
}

func (a *Attackable) stopAggroHateLocked(target attackable.Combatant) {
	if target == nil || a.threats.IsEmpty() {
		return
	}
	a.threats.StopHate(target)
	a.desires.Remove(IntentionAttack, target)
	a.maybeBackToPeaceLocked()
}

func (a *Attackable) reduceAllAggroHateLocked(amount float64) {
	if a.threats.IsEmpty() {
		return
	}
	a.threats.ReduceAllHate(amount)
	a.maybeBackToPeaceLocked()
}

// CurrentIntention returns the currently active intention kind.
func (a *Attackable) CurrentIntention() Intention {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current.kind
}

// TopDesireTarget is the creature the currently executing attack or cast
// intention is aimed at. Idle and follow intentions have no top desire
// target.
func (a *Attackable) TopDesireTarget() attackable.Combatant {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.current.kind {
	case IntentionAttack, IntentionCast:
		return a.current.target
	default:
		return nil
	}
}

// NextIntention returns the queued intention, if one exists.
func (a *Attackable) NextIntention() (Intention, attackable.Combatant, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.next.kind == IntentionIdle {
		return IntentionIdle, nil, false
	}
	return a.next.kind, a.next.target, true
}

// Think advances the current intention once. Safe to call from the periodic
// AI task as well as from a movement-arrived or attack-finished hook. A
// non-nil return reports that an intention step ran but a broadcast within
// it failed; the intention itself still advanced.
func (a *Attackable) Think() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.refreshCombatMemory()
	a.pruneDesires()
	a.dropCurrentIfUnqueued()
	if _, ok := a.desires.Peek(); !ok {
		a.queueIdleFollow()
	}
	for attempts := 0; attempts <= maxDesires; attempts++ {
		a.promoteNext()
		switch a.current.kind {
		case IntentionAttack:
			again, err := a.thinkAttack()
			if again {
				continue
			}
			return err
		case IntentionCast:
			again, err := a.thinkCast()
			if again {
				continue
			}
			return err
		case IntentionFollow:
			return a.thinkFollow()
		case IntentionWander:
			a.thinkWander()
		}
		return nil
	}
	return nil
}

func (a *Attackable) promoteNext() {
	if a.current.kind != IntentionIdle && a.current.kind != IntentionFollow {
		return
	}
	desire, ok := a.desires.Peek()
	if !ok {
		return
	}
	next := intention{}
	switch desire.Kind {
	case IntentionAttack:
		next = intention{kind: IntentionAttack, target: desire.FinalTarget}
	case IntentionCast:
		next = intention{kind: IntentionCast, target: desire.FinalTarget, skill: desire.Skill}
	case IntentionFollow:
		next = intention{kind: IntentionFollow, target: desire.FinalTarget}
	default:
		return
	}
	a.lastKind = a.current.kind
	a.current = next
}

// Tick advances the AI clock and applies periodic attack, cast, and
// nothing-desire weight decay.
func (a *Attackable) Tick() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tickOutOfTerritory()

	a.step++
	if a.step%3 != 0 {
		return
	}
	a.refreshCombatMemory()
	a.reduceAllAggroHateLocked(attackHateDecay)
	a.desires.DecreaseWeightByType(IntentionAttack, attackHateDecay)
	a.desires.DecreaseWeightByType(IntentionCast, castDesireDecay)
	a.desires.DecreaseWeightByType(IntentionNothing, nothingDesireDecay)
	a.step = 0
}

// tickOutOfTerritory ports NpcAI.java's out-of-territory fixed-rate task
// (NpcAI.java:298-339): while the owner is outside its spawn territory, it
// periodically stops hate and drops the queued attack desire for any threat
// entry that hasn't dealt damage in staleThreatAge, so a mob that briefly
// loses and reacquires an out-of-territory attacker doesn't keep hate the
// reference would already have expired. It is a no-op back in territory,
// which also resets the sweep counter so a later territory exit restarts
// the cadence, matching the reference cancelling and recreating the task on
// each isInMyTerritory() transition.
func (a *Attackable) tickOutOfTerritory() {
	if a.actor.InTerritory() {
		a.ootStep = 0
		return
	}
	a.ootStep++
	if a.ootStep < staleThreatSweepTicks {
		return
	}
	a.ootStep = 0

	now := a.now()
	for _, t := range a.threats.Snapshot() {
		if now.Sub(t.Timestamp) < staleThreatAge {
			continue
		}
		a.desires.Remove(IntentionAttack, t.Attacker)
		a.threats.StopHate(t.Attacker)
	}
}

func (a *Attackable) pruneDesires() {
	a.desires.RemoveIf(func(d *Desire) bool {
		if d.Kind != IntentionCast {
			return false
		}
		if d.Weight <= 0 {
			return true
		}
		if a.cast == nil {
			return false
		}
		return !a.cast.MeetsHPMPDisabled(d.FinalTarget, d.Skill)
	})
	if a.actor.DenyAIAction() {
		return
	}
	ox, oy, oz, ok := combatantPosition(a.actor)
	if !ok {
		return
	}
	origin := location.Location{X: ox, Y: oy, Z: oz}
	a.desires.RemoveIf(func(d *Desire) bool {
		if d.Kind != IntentionAttack || d.FinalTarget == nil {
			return false
		}
		tx, ty, tz, ok := combatantPosition(d.FinalTarget)
		if !ok {
			return false
		}
		return origin.Distance3D(location.Location{X: tx, Y: ty, Z: tz}) > attackDesireRange
	})
}

func (a *Attackable) dropCurrentIfUnqueued() {
	if a.actor.DenyAIAction() || a.attack.AttackingNow() {
		return
	}
	if a.cast != nil && a.cast.CastingNow() {
		return
	}
	switch a.current.kind {
	case IntentionAttack, IntentionCast:
		probe := &Desire{Kind: a.current.kind, FinalTarget: a.current.target, Skill: a.current.skill}
		if !a.desires.Has(probe) {
			a.current = intention{kind: IntentionIdle}
		}
	}
}

func combatantPosition(c attackable.Combatant) (x, y, z int, ok bool) {
	p, ok := c.(interface{ Position() (int, int, int) })
	if !ok {
		return 0, 0, 0, false
	}
	x, y, z = p.Position()
	return x, y, z, true
}

// thinkAttack advances one IntentionAttack step. The first return reports
// whether Think's caller should immediately re-promote and continue (true)
// or stop for this cycle (false); the second is any broadcast error from a
// synchronous call this step made, only meaningful when the first is false.
func (a *Attackable) thinkAttack() (bool, error) {
	if a.actor.DenyAIAction() {
		return false, nil
	}

	target := a.current.target
	if a.dropLostTarget(target) {
		return true, nil
	}

	following, err := a.move.MaybeStartOffensiveFollow(target, a.actor.PhysicalAttackRange())
	if following {
		return false, err
	}

	if a.attack.BowCoolingDown() || a.attack.AttackingNow() {
		a.next = a.current
		return false, nil
	}

	if !a.attack.CanAttack(target) {
		return false, nil
	}

	stopErr := a.move.Stop()
	attackErr := a.attack.DoAttack(target)
	return false, errors.Join(stopErr, attackErr)
}

// thinkCast advances an IntentionCast desire once it has been promoted to
// the current intention: pre-movement validation, closing distance on the
// target, planting and facing it once the cast animation is long enough to
// warrant it, then the final cast attempt. It mirrors thinkAttack's shape
// for skill casts instead of physical attacks.
func (a *Attackable) thinkCast() (bool, error) {
	if a.actor.DenyAIAction() || a.cast == nil {
		return false, nil
	}
	if a.cast.Disabled() {
		return false, nil
	}

	target := a.current.target
	ref := a.current.skill
	if a.dropLostCastTarget(target, a.cast.SkillType(ref)) {
		return true, nil
	}

	if !a.cast.CanAttempt(target, ref) {
		return false, nil
	}

	following, err := a.move.MaybeStartOffensiveFollow(target, a.cast.Range(ref))
	if following {
		return false, err
	}

	var stopErr error
	if a.cast.StopsMovement(ref) {
		stopErr = a.move.Stop()
		if target.ObjectID() != a.actor.ObjectID() {
			a.actor.SetHeadingTo(target)
		}
	}

	if !a.cast.CanCast(target, ref) {
		var pawnErr error
		if target.ObjectID() != a.actor.ObjectID() {
			pawnErr = a.actor.BroadcastMoveToPawn(target)
		}
		return false, errors.Join(stopErr, pawnErr)
	}

	a.cast.Cast(target, ref)
	return false, stopErr
}

func (a *Attackable) thinkWander() {
	if a.actor.IsMoving() {
		return
	}
	if a.actor.ReturnHome() {
		return
	}
	if !a.actor.InTerritory() {
		a.current = intention{kind: IntentionIdle}
	}
}

func (a *Attackable) refreshCombatMemory() {
	if a.threats.IsEmpty() {
		a.desires.RemoveKind(IntentionAttack)
	}
	for _, target := range a.threats.Refresh(a.actor.Knows) {
		a.desires.RemoveFinalTarget(target)
		a.clearIntentionsFor(target)
	}
	for _, target := range a.hates.Refresh(a.actor.Knows) {
		a.desires.RemoveFinalTarget(target)
		a.clearIntentionsFor(target)
	}
}

func (a *Attackable) dropLostTarget(target attackable.Combatant) bool {
	return a.dropLostCastTarget(target, "")
}

// dropLostCastTarget is dropLostTarget's cast-path variant: a SUMMON_FRIEND
// cast's target is exempt from the "not known" drop, mirroring the
// reference's isTargetLost(target, skill) rotation-target bypass.
func (a *Attackable) dropLostCastTarget(target attackable.Combatant, skillType string) bool {
	if target == nil {
		a.current = intention{kind: IntentionIdle}
		return true
	}
	if target.AlikeDead() {
		a.threats.StopHate(target)
		a.hates.StopHate(target)
		a.desires.RemoveFinalTarget(target)
		a.clearIntentionsFor(target)
		return true
	}
	if skillType == "SUMMON_FRIEND" {
		return false
	}
	if !a.actor.Knows(target) {
		a.threats.Remove(target)
		a.hates.StopHate(target)
		a.desires.RemoveFinalTarget(target)
		a.clearIntentionsFor(target)
		return true
	}
	return false
}

func (a *Attackable) clearIntentionsFor(target attackable.Combatant) {
	if sameCombatant(a.current.target, target) {
		a.current = intention{kind: IntentionIdle}
	}
	if sameCombatant(a.next.target, target) {
		a.next = intention{}
	}
}
