package ai

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const (
	attackHateDecay    = 6.6
	castDesireDecay    = 66000
	nothingDesireDecay = 0.5
)

// attackDesireRange is the 3D distance past which a queued ATTACK desire
// is dropped when the actor can still choose a new intention.
const attackDesireRange = 1500

// staleThreatAge is the out-of-territory stale-hate threshold: a threat
// entry whose last damage is at least this old gets its hate stopped and
// its queued attack desire dropped while the owner is out of territory.
const staleThreatAge = 90 * time.Second

// ootSweepInitialDelay is the delay before the first out-of-territory
// stale-hate sweep after an arrival that found the owner outside territory.
const ootSweepInitialDelay = 100 * time.Millisecond

// ootSweepPeriod is the interval between later out-of-territory stale-hate
// sweeps. In-territory firings skip the sweep but keep this phase.
const ootSweepPeriod = 10 * time.Second

// AttackableActor is the actor state used by the hostile NPC intention loop.
type AttackableActor interface {
	attackable.Combatant
	DenyAIAction() bool
	// OutOfControl reports whether the actor cannot choose a new intention:
	// DenyAIAction or confused. Desire selection waits while it holds.
	OutOfControl() bool
	PhysicalAttackRange() int
	ReturnHome() bool
	InTerritory() bool
	// SetHeadingTo faces the actor toward target, used before committing to
	// a skill cast whose animation is long enough to plant first.
	SetHeadingTo(attackable.Combatant)
	// BroadcastMoveToPawn sends a rotation-only MoveToPawn notice toward
	// target, used when a final cast attempt is rejected after movement so
	// observers still see the actor face its target.
	BroadcastMoveToPawn(target attackable.Combatant)

	// IdleFollowTarget is the master an escorting NPC follows when idle; nil
	// when it escorts no one.
	IdleFollowTarget() attackable.Combatant
	// ThinkFollow runs one escort-follow step and reports whether the follow
	// desire should be cleared.
	ThinkFollow(target attackable.Combatant, lastWasFollow bool) (clearDesire bool)
	ShouldIdleWander() bool
	ForceWalkStance()
	ForceRunStance()
	RestoreSpawnHeadingIfAtHome()
	RealMoveSpeed() float64
	MoveFromSpawnUsingRandomOffset(offset int)
	// Now reads the clock the actor's queue runs on; hate and desire
	// stamps use it.
	Now() time.Time
	// After runs fn as a task on the actor's queue once d has elapsed on
	// that clock, unless the returned timer is stopped first. The wander
	// chain's firings use it.
	After(d time.Duration, fn func()) Timer
	// AtHookPoint gives the behavior bound to the actor's template its turn
	// at p, on the goroutine that reached p, with the AI loop unlocked (see
	// HookPoint).
	AtHookPoint(p HookPoint)
}

// Timer is a pending one-shot task armed by AttackableActor.After.
type Timer interface {
	// Stop cancels the task and reports whether this call kept it from
	// running.
	Stop() bool
}

// MoveController controls movement requests emitted by the AI loop.
type MoveController interface {
	MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error)
	MoveHome(location.Location) error
	MoveToLocation(location.Location) (bool, error)
	// CanMoveTo reports whether a path to the destination exists.
	CanMoveTo(location.Location) bool
	// CancelFollow drops any follow task without stopping a walk under
	// way, so the old intention's chase cannot pull the actor back.
	CancelFollow()
	Stop()
}

// AttackController controls attack requests emitted by the AI loop.
type AttackController interface {
	BowCoolingDown() bool
	AttackingNow() bool
	CanAttack(attackable.Combatant) bool
	DoAttack(attackable.Combatant)
	// Stop aborts an in-flight attack, including a pending hit task.
	Stop()
}

// hitAnimationReporter is the optional AttackController capability that
// reports the local hit animation window. While it is open, no new desire is
// promoted.
type hitAnimationReporter interface {
	InHitAnimation() bool
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
	// casts a target-lost bypass (the rotation-target exemption).
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
	// Stop aborts an in-flight cast.
	Stop()
}

type intention struct {
	kind   Intention
	target attackable.Combatant
	skill  skill.Ref
	// ctrl is a summon cast's CTRL (forced-use) modifier.
	ctrl  bool
	loc   location.Location
	timer int
	// moveToTarget is an attack or cast intention's desire to close in on
	// its target; every other kind never moves toward one.
	moveToTarget bool
}

// Attackable drives one hostile NPC's combat and wander intentions.
//
// One AI loop owns the current and next intentions. Threat and hate tables,
// and the attack desire queue, are internally synchronized so combat code
// can raise hate while the loop reads target selection. mu guards
// current/next/step: the AI task posts Think and Tick to the NPC's queue,
// where the movement and attack hooks also run, but the first attack desire
// against an actor with no most-hated target calls RunAI from the
// attacker's queue (thinkIfNoMostHated) so the reaction does not wait for
// the next tick. Entry points must serialize against each other. A cast's
// end reaches ClearCurrentDesire synchronously from the cast controller, so
// code holding mu stops the cast controller only while no cast is in
// flight: the idle aborts and every intention step first check castingNow.
// A pass releases mu at each HookPoint, so another entry point may run a
// whole pass there; the pass re-reads what mu guards after each point.
type Attackable struct {
	actor  AttackableActor
	move   MoveController
	attack AttackController
	// hitAnimation is attack's hit-animation report, nil when attack does
	// not track one.
	hitAnimation hitAnimationReporter
	threats      *attackable.ThreatTable
	hates        *attackable.HateTable
	desires      *DesireQueue

	// cast is read without mu so AbortAll can run from inside the think
	// loop, which holds mu (a return-home teleport aborts the actor).
	cast atomic.Pointer[CastController]

	// currentMovesToTarget mirrors current.moveToTarget for the movement
	// controller, which reads it while Think holds mu.
	currentMovesToTarget atomic.Bool

	mu      sync.Mutex
	current intention
	next    intention
	step    int
	// ootSweep is armed only by Arrived while the owner is out of territory
	// and cancelled by Arrived back in territory. Tick never arms or
	// cancels it, so a stationary out-of-territory mob does not decay hate
	// and a brief in-territory tick does not reset the sweep phase.
	ootSweep     bool
	nextOOTSweep time.Time
	// lastKind is the intention kind Think was running before the latest
	// promote, used so escort follow can tell a fresh FOLLOW from a
	// continuing one.
	lastKind Intention
	// passStep is the AI clock value the running think pass reads: for a
	// periodic pass, step as it was before that cycle's Tick advanced it;
	// for any other pass, step as it stands between cycles. Escort follow
	// moves only when it is odd, so an out-of-band pass never shifts the
	// follow rhythm.
	passStep int
	// tickStep is step before the latest Tick advanced it, and ticked
	// reports that no periodic pass has consumed it yet.
	tickStep int
	ticked   bool

	// now returns the current time, the actor's queue clock by default;
	// tests replace it to simulate staleThreatAge elapsing without a real
	// 90-second wait.
	now func() time.Time

	// lastDesire is the intention kind of the last executed desire, used so
	// the first wander step walks immediately and a later promotion starts
	// the wander chain instead.
	lastDesire Intention
	// latched is the one-pass attack latch. An attack promoted while the
	// last executed desire was idle or wander is latched, and the next
	// promotion pass takes it instead of the queue's heaviest desire, even
	// after its desire left the queue; that pass clears it again. While it
	// is set the empty-queue idle waits. kind is IntentionIdle when unset.
	// A respawn builds a new loop, so it starts unset.
	latched intention
	// wanderTask is the wander chain's pending firing, nil when no chain
	// runs. The chain is a self-rescheduling task on the actor's queue,
	// independent of desire selection: each firing rolls randomWalkRate,
	// walks and ends on a hit, and schedules the next firing on a miss.
	// wanderSeq identifies the pending firing, so one that already left the
	// timer when the chain was stopped or replaced does nothing.
	wanderTask Timer
	wanderSeq  uint64
	// randomWalkRate is npcs.properties RandomWalkRate (percent, 0-100).
	randomWalkRate int
	// roll draws a uniform integer in [0, n) for the wander-rate check.
	roll func(n int) int
	// lifeTime is the number of completed periodic AI cycles. Empty-queue
	// idle abort and non-attack promotion run only after the first cycle.
	// A queued ATTACK desire opens the first-cycle promotion gate.
	lifeTime int
}

// NewAttackable builds an idle hostile NPC AI loop.
func NewAttackable(actor AttackableActor, move MoveController, attack AttackController) *Attackable {
	a := &Attackable{
		actor:          actor,
		move:           move,
		attack:         attack,
		hates:          attackable.NewHateTable(actor),
		desires:        NewDesireQueue(),
		current:        intention{kind: IntentionIdle},
		now:            actor.Now,
		randomWalkRate: defaultRandomWalkRate,
		roll:           rnd.Get,
	}
	a.hitAnimation, _ = attack.(hitAnimationReporter)
	a.threats = attackable.NewThreatTable(actor, func() time.Time { return a.now() })
	return a
}

// SetRandomWalkRate records npcs.properties RandomWalkRate for subsequent
// wander chain rolls. The first wander step after idle or combat always
// walks.
func (a *Attackable) SetRandomWalkRate(rate int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.randomWalkRate = rate
}

// MaybeStartOffensiveFollow starts or maintains an offensive follow task
// toward target at this actor's own physical attack range. Exposed for
// AutoAttackTargetValid's queued-desire follow gate, called outside the AI
// loop's own Think step; it must not take a.mu, since
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
	a.cast.Store(&controller)
}

// AbortAll stops movement, the attack cycle and any in-flight cast, in that
// order. Intentions and desires are left as they are: the crowd-control
// state that asked for the abort keeps the think loop from acting on them.
func (a *Attackable) AbortAll() {
	cast := a.CastController()
	a.move.Stop()
	a.attack.Stop()
	if cast != nil {
		cast.Stop()
	}
}

// StopAttack stops the attack cycle; intentions are left as they are.
func (a *Attackable) StopAttack() { a.attack.Stop() }

// CastController returns the currently wired skill-cast handler, or nil if
// none was set. Exposed alongside Threats/Hates/Desires so tests can drive
// or inspect the cast wiring directly instead of through full aggro
// decision-making.
func (a *Attackable) CastController() CastController {
	if cast := a.cast.Load(); cast != nil {
		return *cast
	}
	return nil
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
// this actor should cast a skill (e.g. a monster AI script), and the next
// RunAI / TickThink desire selection (promoteAndStep) promotes whichever
// queued desire currently outweighs the rest. Think only continues the
// current intention and never takes a queued desire up.
func (a *Attackable) Desires() *DesireQueue {
	return a.desires
}

// AddDamageHate records an attacker in the physical threat table.
func (a *Attackable) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	a.threats.AddDamage(attacker, damage, hate)
}

// AddCombatDamageHate records attacker's raw combat damage in the physical
// threat table at zero hate weight — hate is never derived from damage
// dealt here. weight is the ATTACKED-event attack Desire queued alongside
// it, which feeds its own hate into the same threat table (see
// addAttackDesireWithMove) — a separate attack-desire add made by the NPC's
// assigned individual AI script when it is attacked (e.g. the Warrior
// script).
// That per-script formula lives in the domain layer (see
// Hostile.attackedHateWeight), not here: this generic AI plumbing only
// applies whatever weight it is given. When the threat table had no
// most-hated attacker, the AI loop runs immediately so the first reaction
// does not wait for the next tick.
func (a *Attackable) AddCombatDamageHate(attacker attackable.Combatant, damage, weight float64) {
	_, hadMostHated := a.threats.MostHated()
	a.threats.AddDamage(attacker, damage, 0)
	if attacker == nil || (a.actor.SiegeGuard() && attacker.SiegeGuard()) {
		return
	}
	a.addAttackDesire(attacker, weight)
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

// addAttackDesireWithMove queues the attack Desire and, since an attack
// desire updates aggro by default (every public entry point here uses that
// default; none use the no-aggro-update variant), feeds the same weight
// into the physical threat table at zero extra damage — the paired
// hate-list add.
func (a *Attackable) addAttackDesireWithMove(attacker attackable.Combatant, hate float64, moveToTarget bool) {
	a.queueAttackDesireOnly(attacker, hate, moveToTarget)
	a.threats.AddDamage(attacker, 0, hate)
}

// addAttackDesireNoAggro queues the attack Desire without touching the
// threat table — the hate-randomizing rebuild's own no-aggro-update desire
// add — used
// to resync the Desire queue from a threat table whose hate is already
// authoritative (RandomizeHate) instead of adding to it a second time.
func (a *Attackable) addAttackDesireNoAggro(attacker attackable.Combatant, hate float64) {
	a.queueAttackDesireOnly(attacker, hate, true)
}

func (a *Attackable) queueAttackDesireOnly(attacker attackable.Combatant, hate float64, moveToTarget bool) {
	a.desires.AddOrUpdate(&Desire{
		Kind:         IntentionAttack,
		FinalTarget:  attacker,
		Weight:       hate,
		QueuedAt:     a.now(),
		MoveToTarget: moveToTarget,
	})
}

const escortFollowWeight = 5

// defaultWanderTimer is addWanderDesire's timer argument in seconds.
const defaultWanderTimer = 5

// defaultWanderWeight is addWanderDesire's weight argument.
const defaultWanderWeight = 5

// defaultRandomWalkRate is npcs.properties RandomWalkRate's shipped default.
const defaultRandomWalkRate = 30

// AddMoveToDesire queues a weighted MOVE_TO request and reports whether it
// was accepted. It does not take the AI mutex so ReturnHome can enqueue
// from thinkWander, which already holds it. A movement-disabled actor, or
// one whose move controller reports the destination unreachable, drops the
// request.
func (a *Attackable) AddMoveToDesire(loc location.Location, weight float64) bool {
	if a.actor.MovementDisabled() || !a.move.CanMoveTo(loc) {
		return false
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:     IntentionMoveTo,
		Location: loc,
		Weight:   weight,
		QueuedAt: a.now(),
	})
	return true
}

func (a *Attackable) addFollowDesire(target attackable.Combatant, weight float64) {
	if target == nil {
		return
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:        IntentionFollow,
		FinalTarget: target,
		Weight:      weight,
		QueuedAt:    a.now(),
	})
}

// thinkIdle aborts every action, goes idle and opens the no-desire point.
func (a *Attackable) thinkIdle() {
	a.move.Stop()
	a.attack.Stop()
	if cast := a.CastController(); cast != nil {
		cast.Stop()
	}
	a.actor.ForceWalkStance()
	a.setCurrent(intention{kind: IntentionIdle})
	a.atHookPoint(HookNoDesire)
}

func (a *Attackable) queueIdleFollow() {
	a.addFollowDesire(a.actor.IdleFollowTarget(), escortFollowWeight)
}

func (a *Attackable) queueIdleWander() {
	if !a.actor.ShouldIdleWander() {
		return
	}
	a.desires.AddOrUpdate(&Desire{
		Kind:     IntentionWander,
		Timer:    defaultWanderTimer,
		Weight:   defaultWanderWeight,
		QueuedAt: a.now(),
	})
}

func (a *Attackable) thinkFollow() error {
	if a.passStep%2 == 0 {
		return nil
	}
	if a.actor.ThinkFollow(a.current.target, a.lastKind == IntentionFollow) {
		a.desires.Remove(IntentionFollow, a.current.target)
		a.setCurrent(intention{kind: IntentionIdle})
	}
	return nil
}

// thinkIfNoMostHated re-runs desire selection (RunAI) immediately for a
// first attack desire. An attacker the actor does not know is skipped
// because the attack step drops unknown targets, so an unseen target keeps
// its queued desire for a later tick instead of being wiped here.
func (a *Attackable) thinkIfNoMostHated(hadMostHated bool, attacker attackable.Combatant) {
	if hadMostHated || attacker == nil || !a.actor.Knows(attacker) {
		return
	}
	_ = a.RunAI()
}

// RandomizeHate is the AI side of attack randomization, driving the
// randomize-hate effect: swaps a random valid attacker into the most-hated
// slot ahead of the current target (see ThreatTable.RandomizeAttack), then
// clears and rebuilds the queued attack desires from every threat entry so
// they match the post-swap hate table, requeued without updating aggro.
// Reports whether a swap happened.
func (a *Attackable) RandomizeHate(valid func(attackable.Combatant) bool, pick func(int) int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.threats.RandomizeAttack(valid, pick) {
		return false
	}

	a.desires.RemoveKind(IntentionAttack)
	for _, t := range a.threats.Snapshot() {
		a.addAttackDesireNoAggro(t.Attacker, t.Hate)
	}
	return true
}

// ReconsiderTarget is the AI side of in-range target reconsideration,
// used when this actor can no longer act on its current target (e.g. an
// immobilize state): swaps in a replacement from the threat table (see
// ThreatTable.ReconsiderTarget) and drops the previous most-hated attacker's
// queued attack desire if one existed. It never queues a desire for the
// chosen target — the hate-list reconsideration only stops hate and adds
// zero hate on the list, never touches the caller's desire queue; that is
// left to a future caller, matching RandomizeHate's sibling behavior of
// leaving target acquisition outside the reconsideration's scope. Reports the new
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

// SetBackToPeace clears combat memory and cancels the current action. It
// does not start a return-home walk; an out-of-territory owner stays idle
// until a later Think queues a new desire.
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
	a.setCurrent(intention{kind: IntentionIdle})
	a.stopWanderChain()
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

// CurrentIntentionMovesToTarget reports whether the current intention may
// close in on its target: an attack or cast queued to move rather than
// hold. Safe to call while the AI loop runs.
func (a *Attackable) CurrentIntentionMovesToTarget() bool {
	return a.currentMovesToTarget.Load()
}

// setCurrent replaces the current intention. Callers hold mu.
func (a *Attackable) setCurrent(next intention) {
	a.current = next
	a.currentMovesToTarget.Store(next.moveToTarget)
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

// thinkMode selects which entry point a think pass serves.
type thinkMode uint8

const (
	// thinkContinue continues the current intention after a bow reuse
	// ending or a control effect ending; it never selects a desire and
	// never idles on an empty queue.
	thinkContinue thinkMode = iota
	// thinkEvent re-runs desire selection on an event: the hit animation
	// ending, a bow shot, a completed cast or a first attack desire.
	thinkEvent
	// thinkTick is the periodic AI cycle.
	thinkTick
	// thinkAttackFinished is a swing finishing: desire selection as on an
	// event, then, for an out-of-control actor that selects nothing, the
	// continue pass the finished attack's think falls back to.
	thinkAttackFinished
)

// Think advances the current intention once, for a bow's reuse ending and
// a control effect ending; an arrival does not think. It never selects a
// queued desire, even from idle, follow or wander, and never idles a busy
// actor on an empty queue: RunAI and TickThink do both. An actor already
// idle repeats its idle step and opens the no-desire point. A walk an arrival
// finished is still current, so Think steps it again; a wander it finished
// takes no step. A non-nil return
// reports that an intention step ran but a broadcast within it failed; the
// intention itself still advanced.
func (a *Attackable) Think() error {
	return a.think(thinkContinue)
}

// RunAI re-runs desire selection on an event. After the first periodic
// cycle, an actor that is not casting, has an empty desire queue and is
// not idle aborts everything and goes idle; otherwise it makes the
// heaviest queued desire current and steps it. An out-of-control actor
// does neither.
func (a *Attackable) RunAI() error {
	return a.think(thinkEvent)
}

// TickThink is the periodic AI cycle. Empty-queue idle abort runs after
// the first cycle and does not promote a follow or wander queued in that
// same cycle. Promotion itself also waits for that first cycle unless an
// ATTACK desire is already queued, and is skipped while a cast is in flight
// or the actor is out of control.
func (a *Attackable) TickThink() error {
	return a.think(thinkTick)
}

// AttackFinished is a swing finishing: RunAI's desire selection, followed
// by the finished attack's think. An in-control actor's selection already
// stepped the current intention, so only an out-of-control one, which
// selects nothing, takes that think as a continue pass: a confused actor
// keeps swinging at its current target while a stunned one does nothing.
func (a *Attackable) AttackFinished() error {
	return a.think(thinkAttackFinished)
}

// ClearCurrentDesire drops the queued desire matching the current intention,
// leaving the intention itself in place. A finished or aborted cast calls it
// so its CAST desire is not picked again.
func (a *Attackable) ClearCurrentDesire() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clearCurrentDesire()
}

func (a *Attackable) castingNow() bool {
	cast := a.CastController()
	return cast != nil && cast.CastingNow()
}

func (a *Attackable) canPromote(updateTick bool, instantRun bool) bool {
	if a.castingNow() {
		return false
	}
	if !updateTick {
		return true
	}
	return a.lifeTime > 0 || instantRun
}

func (a *Attackable) think(mode thinkMode) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	updateTick := mode == thinkTick
	// Only an actor idle when the pass starts repeats its idle step on a
	// continue pass; one this pass drops to idle takes no step.
	idleAtStart := a.current.kind == IntentionIdle
	// The first-cycle promotion gate is decided before the see-creature
	// point: an attack desire queued there does not open it.
	instantRun := a.lifeTime == 0 && a.desires.hasKind(IntentionAttack)
	if updateTick {
		a.atHookPoint(HookSeeCreature)
	}
	a.passStep = a.step
	if updateTick && a.ticked {
		a.passStep = a.tickStep
		a.ticked = false
	}
	outOfControl := a.actor.OutOfControl()
	a.refreshCombatMemory()
	a.pruneDesires(outOfControl)
	onEvent := mode == thinkEvent || mode == thinkAttackFinished
	if onEvent && !outOfControl && a.idleOnEmptyQueue() {
		return nil
	}
	if !outOfControl {
		a.dropCurrentIfUnqueued(mode)
	}
	canPromote := a.canPromote(updateTick, instantRun)
	// idleAfterLatch is the periodic cycle's empty-queue idle deferred past
	// a latched attack: the latched step runs first and the idle then
	// aborts it, as the cycle's idle does not wait for the latch.
	idleAfterLatch := false
	if updateTick {
		idled := false
		if _, ok := a.desires.Peek(); !ok {
			if a.lifeTime > 0 && !a.castingNow() {
				if a.hasLatch() {
					idleAfterLatch = true
				} else {
					a.thinkIdle()
					a.queueIdleFollow()
					idled = true
				}
			}
		}
		if _, ok := a.desires.Peek(); !ok {
			if a.lifeTime > 0 && !a.castingNow() && !idleAfterLatch {
				a.queueIdleWander()
			}
		}
		a.lifeTime++
		if idled {
			return nil
		}
	}
	// An out-of-control actor selects nothing: the current intention, the
	// attack latch and lastDesire stay as they are, and a queued wander
	// takes no step. Only the periodic cycle's empty-queue idle runs, at
	// once even with a latch set; the latch waits for the first pass after
	// control returns. A finished swing still continues the current
	// intention, as its think does not go through desire selection.
	if outOfControl && mode != thinkContinue {
		if idleAfterLatch {
			a.idleAndRequeue()
		}
		if mode == thinkAttackFinished && canPromote {
			return a.continueCurrent(idleAtStart)
		}
		return nil
	}
	if !canPromote {
		return nil
	}
	if mode == thinkContinue {
		return a.continueCurrent(idleAtStart)
	}
	err := a.promoteAndStep()
	if idleAfterLatch {
		if _, ok := a.desires.Peek(); !ok && !a.castingNow() {
			a.idleAndRequeue()
		}
	}
	return err
}

// idleAndRequeue is the empty-queue idle: abort everything, go idle, and
// queue the idle follow, else the idle wander.
func (a *Attackable) idleAndRequeue() {
	a.thinkIdle()
	a.queueIdleFollow()
	if _, ok := a.desires.Peek(); !ok {
		a.queueIdleWander()
	}
}

// continueCurrent is Think's pass: one step of the current intention with
// no desire selection, so a queued desire waits for the next RunAI or
// TickThink. It neither takes nor updates the attack latch, leaves
// lastDesire to desire selection, and does not re-select on a lost target.
// A walk steps even when an arrival already finished it: one that stopped
// short of its destination sets off again. An actor that was idle when the
// pass started (wasIdle) runs the idle step again, so the no-desire point
// opens, but queues no idle follow or wander: that is the periodic cycle's.
// An intention this pass dropped to idle takes no step. The continue pass
// has no wander step.
func (a *Attackable) continueCurrent(wasIdle bool) error {
	switch a.current.kind {
	case IntentionIdle:
		if wasIdle {
			a.thinkIdle()
		}
	case IntentionAttack:
		_, err := a.thinkAttack()
		return err
	case IntentionCast:
		_, err := a.thinkCast()
		return err
	case IntentionFollow:
		return a.thinkFollow()
	case IntentionMoveTo:
		a.thinkMoveTo()
	}
	return nil
}

// promoteAndStep promotes the next desire and runs one step of the current
// intention. A lost attack or cast target re-promotes at once, except for a
// latched attack, which is the whole pass.
func (a *Attackable) promoteAndStep() error {
	for attempts := 0; attempts <= maxDesires; attempts++ {
		latched, promoted := a.promoteNext()
		switch a.current.kind {
		case IntentionAttack:
			again, err := a.thinkAttack()
			if again && !latched {
				continue
			}
			a.lastDesire = IntentionAttack
			return err
		case IntentionCast:
			again, err := a.thinkCast()
			if again {
				continue
			}
			a.lastDesire = IntentionCast
			return err
		case IntentionFollow:
			a.lastDesire = IntentionFollow
			return a.thinkFollow()
		case IntentionWander:
			// A wander steps only on the pass that promotes it; while it
			// stays current, its chain fires on its own.
			if !promoted || !a.currentQueued() {
				return nil
			}
			a.thinkWander()
			a.lastDesire = IntentionWander
		case IntentionMoveTo:
			if !a.currentQueued() {
				return nil
			}
			a.thinkMoveTo()
			a.lastDesire = IntentionMoveTo
		}
		return nil
	}
	return nil
}

// currentQueued reports whether the current wander or walk still has its
// desire queued. Desire selection steps a walk or wander only through its
// queued desire, so one an arrival finished waits, still current, for the
// idle or the next promotion instead of setting off again.
func (a *Attackable) currentQueued() bool {
	return a.desires.Has(&Desire{Kind: a.current.kind, Location: a.current.loc})
}

// idleOnEmptyQueue runs the event-driven empty-queue idle: once the first
// periodic cycle has run, an actor that is not casting, has no queued
// desire and is not already idle aborts everything and goes idle. The
// caller skips it while the actor is out of control. It reports whether it
// did; the idle then takes no further step this pass.
func (a *Attackable) idleOnEmptyQueue() bool {
	if a.lifeTime == 0 || a.current.kind == IntentionIdle || a.hasLatch() || a.castingNow() {
		return false
	}
	if _, ok := a.desires.Peek(); ok {
		return false
	}
	a.idleAndRequeue()
	return true
}

// inHitAnimation reports whether the attack's hit animation window is open.
func (a *Attackable) inHitAnimation() bool {
	return a.hitAnimation != nil && a.hitAnimation.InHitAnimation()
}

// hasLatch reports whether a latched attack waits for the next pass.
func (a *Attackable) hasLatch() bool {
	return a.latched.kind != IntentionIdle
}

// promoteNext picks the latched attack, else the heaviest queued desire,
// and updates the latch from it. It reports whether the pick was the
// latched attack, and whether anything was promoted: nothing is during the
// hit animation, with no promotable desire, or when a wander is current
// and the pick is a wander too.
//
// Desire selection always makes the pick current, whatever the current
// intention is, so a heavier attack on another target, a cast or a walk
// takes over a running attack. Replacing the intention with a different
// one drops any follow task and the queued next intention the old one left
// behind.
func (a *Attackable) promoteNext() (fromLatch, promoted bool) {
	if a.inHitAnimation() {
		return false, false
	}
	next, fromLatch, ok := a.nextToDo()
	if !ok {
		return false, false
	}
	if a.current.kind == IntentionWander && next.kind == IntentionWander {
		return false, false
	}
	if next.kind == IntentionAttack && (a.lastDesire == IntentionIdle || a.lastDesire == IntentionWander) {
		a.latched = next
	} else {
		a.latched = intention{}
	}
	if !a.current.same(next) {
		a.move.CancelFollow()
		a.next = intention{}
	}
	a.lastKind = a.current.kind
	a.setCurrent(next)
	return fromLatch, true
}

// same reports whether o is the same intention as i: same kind, aimed at
// the same target with the same skill, or walking to the same location.
func (i intention) same(o intention) bool {
	return i.kind == o.kind && sameCombatant(i.target, o.target) && i.skill == o.skill && i.loc == o.loc
}

// nextToDo returns the latched attack when one is set, else the heaviest
// queued desire as an intention. ok is false when there is neither or the
// heaviest desire's kind is not promotable.
func (a *Attackable) nextToDo() (next intention, fromLatch, ok bool) {
	if a.hasLatch() {
		return a.latched, true, true
	}
	desire, ok := a.desires.Peek()
	if !ok {
		return intention{}, false, false
	}
	switch desire.Kind {
	case IntentionAttack:
		next = intention{kind: IntentionAttack, target: desire.FinalTarget, moveToTarget: desire.MoveToTarget}
	case IntentionCast:
		next = intention{kind: IntentionCast, target: desire.FinalTarget, skill: desire.Skill, moveToTarget: desire.MoveToTarget}
	case IntentionFollow:
		next = intention{kind: IntentionFollow, target: desire.FinalTarget}
	case IntentionWander:
		next = intention{kind: IntentionWander, timer: desire.Timer}
	case IntentionMoveTo:
		next = intention{kind: IntentionMoveTo, loc: desire.Location}
	default:
		return intention{}, false, false
	}
	return next, false, true
}

// Tick advances the AI clock and applies periodic attack, cast, and
// nothing-desire weight decay.
func (a *Attackable) Tick() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tickOutOfTerritory()

	a.tickStep, a.ticked = a.step, true
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

// tickOutOfTerritory runs the arrival-armed out-of-territory stale-hate
// sweep. Arrived starts the schedule (100ms then every 10s) when the owner
// is outside territory and cancels it on an in-territory arrival. A Tick
// while in territory skips that firing but keeps the phase, so oscillating
// across the territory edge still sweeps. A stationary out-of-territory
// owner that never arrives never arms the sweep.
func (a *Attackable) tickOutOfTerritory() {
	if !a.ootSweep {
		return
	}
	now := a.now()
	if now.Before(a.nextOOTSweep) {
		return
	}
	for !a.nextOOTSweep.After(now) {
		a.nextOOTSweep = a.nextOOTSweep.Add(ootSweepPeriod)
	}
	if a.actor.InTerritory() {
		return
	}

	for _, t := range a.threats.Snapshot() {
		if now.Sub(t.Timestamp) < staleThreatAge {
			continue
		}
		a.desires.Remove(IntentionAttack, t.Attacker)
		a.threats.StopHate(t.Attacker)
	}
}

// syncOOTSweepLocked disarms the out-of-territory stale-hate sweep on an
// arrival inside territory, and arms it on the first one outside, after
// the out-of-territory point.
func (a *Attackable) syncOOTSweepLocked() {
	if a.actor.InTerritory() {
		a.ootSweep = false
		a.nextOOTSweep = time.Time{}
		return
	}
	if a.ootSweep {
		return
	}
	a.atHookPoint(HookOutOfTerritory)
	a.ootSweep = true
	a.nextOOTSweep = a.now().Add(ootSweepInitialDelay)
}

// pruneDesires drops invalid cast desires, then, unless the actor is out of
// control, attack desires whose target is beyond attackDesireRange.
func (a *Attackable) pruneDesires(outOfControl bool) {
	a.desires.RemoveIf(func(d *Desire) bool {
		if d.Kind != IntentionCast {
			return false
		}
		if d.Weight <= 0 {
			return true
		}
		cast := a.CastController()
		return cast != nil && !cast.MeetsHPMPDisabled(d.FinalTarget, d.Skill)
	})
	if outOfControl {
		return
	}
	ox, oy, oz := a.actor.Position()
	origin := location.Location{X: ox, Y: oy, Z: oz}
	a.desires.RemoveIf(func(d *Desire) bool {
		if d.Kind != IntentionAttack || d.FinalTarget == nil {
			return false
		}
		tx, ty, tz := d.FinalTarget.Position()
		return origin.Distance3D(location.Location{X: tx, Y: ty, Z: tz}) > attackDesireRange
	})
}

// dropCurrentIfUnqueued idles an attack, cast or walk whose desire is no
// longer queued, unless an action is in flight. The caller skips it while
// the actor is out of control. The continue pass keeps a desire-less walk:
// it steps the current walk as it stands, so one an arrival left short of
// its destination sets off again.
func (a *Attackable) dropCurrentIfUnqueued(mode thinkMode) {
	if a.attack.AttackingNow() {
		return
	}
	if a.castingNow() {
		return
	}
	switch a.current.kind {
	case IntentionAttack, IntentionCast:
		probe := &Desire{Kind: a.current.kind, FinalTarget: a.current.target, Skill: a.current.skill}
		if !a.desires.Has(probe) {
			a.setCurrent(intention{kind: IntentionIdle})
		}
	case IntentionMoveTo:
		if mode == thinkContinue {
			return
		}
		probe := &Desire{Kind: IntentionMoveTo, Location: a.current.loc}
		if !a.desires.Has(probe) {
			a.setCurrent(intention{kind: IntentionIdle})
		}
	}
}

// thinkAttack advances one IntentionAttack step. The first return reports
// whether RunAI / TickThink desire selection (promoteAndStep) should
// immediately re-promote and continue (true) or stop for this cycle (false);
// continueCurrent (Think) ignores it and never re-selects. The second is any
// broadcast error from a synchronous call this step made, only meaningful
// when the first is false.
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

	a.move.Stop()
	a.attack.DoAttack(target)
	return false, nil
}

// thinkCast advances an IntentionCast desire once it has been promoted to
// the current intention: pre-movement validation, closing distance on the
// target, planting and facing it once the cast animation is long enough to
// warrant it, then the final cast attempt. It mirrors thinkAttack's shape
// for skill casts instead of physical attacks.
func (a *Attackable) thinkCast() (bool, error) {
	cast := a.CastController()
	if a.actor.DenyAIAction() || cast == nil {
		return false, nil
	}
	if cast.Disabled() {
		return false, nil
	}

	target := a.current.target
	ref := a.current.skill
	if a.dropLostCastTarget(target, cast.SkillType(ref)) {
		return true, nil
	}

	if !cast.CanAttempt(target, ref) {
		return false, nil
	}

	following, err := a.move.MaybeStartOffensiveFollow(target, cast.Range(ref))
	if following {
		a.actor.ForceRunStance()
		return false, err
	}

	if cast.StopsMovement(ref) {
		a.move.Stop()
		if target.ObjectID() != a.actor.ObjectID() {
			a.actor.SetHeadingTo(target)
		}
	}

	if !cast.CanCast(target, ref) {
		if target.ObjectID() != a.actor.ObjectID() {
			a.actor.BroadcastMoveToPawn(target)
		}
		return false, nil
	}

	cast.Cast(target, ref)
	return false, nil
}

func (a *Attackable) thinkMoveTo() {
	if a.actor.DenyAIAction() {
		return
	}
	if a.actor.MovementDisabled() {
		return
	}
	if ox, oy, oz := a.actor.Position(); (location.Location{X: ox, Y: oy, Z: oz}) == a.current.loc {
		a.atHookPoint(HookMoveFinished)
		a.clearCurrentDesire()
		return
	}
	_ = a.move.MoveHome(a.current.loc)
}

// Arrived drops the queued MOVE_TO, FLEE, or WANDER desire when movement
// finishes and leaves the intention current: the next RunAI or TickThink
// idles it on an empty queue or promotes the next desire, and desire
// selection never steps it again without its desire (currentQueued). A
// Think before then (a control effect ending) steps a walk once more and
// leaves a wander alone. Arrival itself never thinks.
// A MOVE_TO or FLEE opens the move-finished point before its desire is
// dropped.
// Escort FOLLOW returns without restoring spawn heading or arming the
// out-of-territory stale-hate sweep; combat chase stays ATTACK and still
// runs both.
func (a *Attackable) Arrived() {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.current.kind {
	case IntentionFollow:
		return
	case IntentionMoveTo, IntentionFlee:
		// IntentionFlee is dormant until promoteNext can make it current.
		a.atHookPoint(HookMoveFinished)
		a.clearCurrentDesire()
	case IntentionWander:
		a.clearCurrentDesire()
	}
	a.actor.RestoreSpawnHeadingIfAtHome()
	a.syncOOTSweepLocked()
}

// ArrivedBlocked drops the queued MOVE_TO, FLEE, or WANDER desire when an
// in-flight walk is stopped by a blocked geodata path, leaving the
// intention current, same as Arrived.
func (a *Attackable) ArrivedBlocked() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.clearArrivalDesire()
}

func (a *Attackable) clearArrivalDesire() {
	switch a.current.kind {
	case IntentionMoveTo, IntentionFlee, IntentionWander:
		// IntentionFlee is dormant until promoteNext can make it current.
		a.clearCurrentDesire()
	}
}

func (a *Attackable) clearCurrentDesire() {
	probe := &Desire{Kind: a.current.kind, Location: a.current.loc, FinalTarget: a.current.target, Skill: a.current.skill}
	a.desires.RemoveIf(func(d *Desire) bool { return d.Equal(probe) })
}

// thinkWander is the wander step: the promotion of a WANDER desire and
// each chain firing that rolled a miss. The first step after a desire of
// another kind ends any running chain and walks at once; a later one
// starts the chain, which waits the wander timer before its first roll.
func (a *Attackable) thinkWander() {
	if a.current.kind != IntentionWander {
		return
	}
	a.actor.ForceWalkStance()
	if a.actor.IsMoving() {
		return
	}
	if a.lastDesire != IntentionWander {
		a.stopWanderChain()
		a.doWanderMove()
		return
	}
	a.scheduleWander()
}

// scheduleWander arms the wander chain's next firing one wander timer from
// now. A chain already pending is kept, so an actor runs one chain at most:
// its firing comes first and would cancel the newer one anyway.
func (a *Attackable) scheduleWander() {
	if a.wanderTask != nil {
		return
	}
	a.wanderSeq++
	seq := a.wanderSeq
	a.wanderTask = a.actor.After(a.wanderDelay(), func() { a.fireWander(seq) })
}

// stopWanderChain cancels the wander chain's pending firing, if any.
func (a *Attackable) stopWanderChain() {
	if a.wanderTask == nil {
		return
	}
	a.wanderTask.Stop()
	a.wanderTask = nil
	a.wanderSeq++
}

// fireWander is one firing of the wander chain, run on the actor's queue
// whatever the actor's control state. A hit walks when the actor still
// wanders and stands, and ends the chain either way: a hit whose walk is
// refused, as for a stunned actor, leaves the wander current with its
// desire queued, and desire selection does not restart a current wander,
// so the actor stands until another desire replaces it or it idles. A miss
// takes the wander step again, which schedules the next firing while the
// wander is still current and standing. A dead actor's chain ends.
func (a *Attackable) fireWander(seq uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if seq != a.wanderSeq {
		return
	}
	a.wanderTask = nil
	if a.actor.AlikeDead() {
		return
	}
	if a.randomWalkRate > 0 && a.roll != nil && a.roll(100) < a.randomWalkRate {
		if !a.actor.IsMoving() && a.current.kind == IntentionWander {
			a.doWanderMove()
		}
		return
	}
	a.thinkWander()
}

func (a *Attackable) wanderDelay() time.Duration {
	timer := a.current.timer
	if timer <= 0 {
		timer = defaultWanderTimer
	}
	return time.Duration(timer) * time.Second
}

func (a *Attackable) doWanderMove() {
	if a.actor.ReturnHome() {
		return
	}
	// Out of territory with no walk home: drop only the wander desire. The
	// wander stays current until desire selection replaces it or idles the
	// actor on the now-empty queue.
	if !a.actor.InTerritory() {
		a.clearCurrentDesire()
		return
	}
	a.actor.MoveFromSpawnUsingRandomOffset(int(a.actor.RealMoveSpeed()) * 3)
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
// cast's target is exempt from the "not known" drop (the rotation-target
// bypass of the target-lost check).
func (a *Attackable) dropLostCastTarget(target attackable.Combatant, skillType string) bool {
	if target == nil {
		a.setCurrent(intention{kind: IntentionIdle})
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
		a.setCurrent(intention{kind: IntentionIdle})
	}
	if sameCombatant(a.next.target, target) {
		a.next = intention{}
	}
}
