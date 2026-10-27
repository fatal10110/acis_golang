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

// ootSweepPeriod is the fixed rate of the later out-of-territory stale-hate
// sweeps. A firing that finds the owner in territory skips its sweep but
// keeps this phase.
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

	// ThinkFollow runs one escort-follow step and reports whether the follow
	// desire should be cleared.
	ThinkFollow(target attackable.Combatant, lastWasFollow bool) (clearDesire bool)
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
	// chain's firings and the out-of-territory stale-hate sweep use it; the
	// queue closing when the actor leaves the world cancels both.
	After(d time.Duration, fn func()) Timer
	// AtHookPoint gives the behavior bound to the actor's template its turn
	// at p, on the goroutine that reached p, with the AI loop unlocked (see
	// HookPoint).
	AtHookPoint(p HookPoint)
	// AISleeping reports whether the AI task leaves the actor alone: it is
	// dead, out of the world, or asleep in an inactive region.
	AISleeping() bool
	// SocialAction shows observers the actor playing social animation id.
	SocialAction(id int)
	// InWorld reports whether target is still in the world: an object that
	// left it (a logged-out player, a decayed NPC) is a lost target even for
	// a cast that does not need the actor to know it.
	InWorld(target attackable.Combatant) bool
	// PickUpTarget returns where the ground item itemObjectID lies, ok
	// false once the actor can no longer pick it up: it left the world or
	// the ground, or the actor does not know it.
	PickUpTarget(itemObjectID int32) (at location.Location, ok bool)
	// PickUp picks up the ground item itemObjectID: it takes the item
	// before any other pickup can, gives the behavior bound to the actor's
	// template its picked-item turn, then takes the item out of the world.
	// The AI loop is unlocked for the call. It reports false, having done
	// nothing, when the item is gone or another pickup holds it.
	PickUp(itemObjectID int32) bool
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

// AttackableMoveController is the MoveController of a hostile NPC's AI
// loop, which also walks to ground items.
type AttackableMoveController interface {
	MoveController
	// MoveStraightToLocation walks straight for a point with path-finding
	// off: a closed line ends the walk blocked instead of being routed
	// around.
	MoveStraightToLocation(location.Location) (bool, error)
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
	// CanDesire runs the gates a cast desire queued with its conditions
	// checked passes: ref's reuse, then the MP and the HP its hit takes.
	CanDesire(target attackable.Combatant, ref skill.Ref) bool
	// FinalTarget resolves the creature ref is cast on given the commanded
	// target (nil when there is none), or nil when the skill has none.
	FinalTarget(target attackable.Combatant, ref skill.Ref) attackable.Combatant
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
	ctrl bool
	// loc is a walk's destination, or where a flee started.
	loc location.Location
	// timer is a wander's chain period in seconds, or a social's hold in
	// milliseconds.
	timer int
	// distance is how far a flee runs from its start.
	distance int
	// itemObjectID is a pick-up's item, or a social's animation id.
	itemObjectID int32
	// moveToTarget is an attack or cast intention's desire to close in on
	// its target; every other kind never moves toward one.
	moveToTarget bool
	// route is a route walk's route name.
	route string
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
	move   AttackableMoveController
	attack AttackController
	// hitAnimation is attack's hit-animation report, nil when attack does
	// not track one.
	hitAnimation hitAnimationReporter
	threats      *attackable.ThreatTable
	hates        *attackable.HateTable
	desires      *DesireQueue
	// route is the walker route route desires walk, nil for none; set once
	// before the actor is published.
	route Route

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
	// territory is the actor's out-of-territory state, kept across its
	// lives: whether its last arrival was outside its territory, and the
	// deadline of the out-of-territory stale-hate sweep's next firing on its
	// fixed-rate grid. The first such arrival arms the sweep, and only an
	// arrival back in territory ends it. ootSweep is this life's pending
	// firing (nil for none), and ootSeq the generation a firing must match
	// to run. The sweep runs on the actor's queue, apart from the AI task: a
	// stationary actor out of territory never arms it, and an inactive
	// region does not stall it.
	territory *TerritoryMemory
	ootSweep  Timer
	ootSeq    uint64
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
	// cycleDecayDue reports that the latest Tick reached every third step:
	// that cycle's hate refresh and weight decay run once its periodic pass
	// has selected, so they shape the next cycle's selection, not this one's.
	cycleDecayDue bool
	// onRoute reports that the actor walks its route: a route intention
	// stepped since the AI last took up another desire.
	onRoute bool

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
	// topDesireTarget is the final target of the desire desire selection
	// last took up, nil for a desire with none. Only the periodic cycle's
	// empty-queue idle clears it; the event idle and a desire leaving the
	// queue leave it as it is.
	topDesireTarget attackable.Combatant
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
	// socialHold is when the last social animation's hold ends: until then
	// no desire is chosen and the event idle waits. The zero time holds
	// nothing; a respawn builds a new loop, so it starts there.
	socialHold time.Time
	// lifeTime is the number of completed periodic AI cycles. Empty-queue
	// idle abort and non-attack promotion run only after the first cycle.
	// A queued ATTACK desire opens the first-cycle promotion gate. think
	// increments it under mu; ResetLifeTime stores 0 at death from the
	// killer's goroutine without mu; LifeTime reads it without a lock.
	lifeTime atomic.Int32
}

// NewAttackable builds an idle hostile NPC AI loop.
func NewAttackable(actor AttackableActor, move AttackableMoveController, attack AttackController) *Attackable {
	a := &Attackable{
		actor:          actor,
		move:           move,
		attack:         attack,
		hates:          attackable.NewHateTable(actor),
		desires:        NewDesireQueue(),
		current:        intention{kind: IntentionIdle},
		territory:      &TerritoryMemory{},
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

// LifeTime returns the number of periodic AI cycles the actor has
// completed since it spawned. It takes no lock.
func (a *Attackable) LifeTime() int32 { return a.lifeTime.Load() }

// ResetLifeTime starts the cycle count over, as a death does.
func (a *Attackable) ResetLifeTime() { a.lifeTime.Store(0) }

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

// AddAttackDesire queues an attack intention that closes on the target.
// When the threat table has no most-hated attacker, the AI loop runs
// immediately so the first reaction does not wait for the next tick.
func (a *Attackable) AddAttackDesire(attacker attackable.Combatant, hate float64) {
	a.queueAttackDesire(attacker, 0, hate, true)
}

// AddAttackDesireHold queues an attack intention that stays in place instead
// of closing on the target. Same first-reaction Think as AddAttackDesire.
func (a *Attackable) AddAttackDesireHold(attacker attackable.Combatant, hate float64) {
	a.queueAttackDesire(attacker, 0, hate, false)
}

// AddAttackDesireDamage is AddAttackDesire that also records damage
// against attacker in the threat table.
func (a *Attackable) AddAttackDesireDamage(attacker attackable.Combatant, damage int, hate float64) {
	a.queueAttackDesire(attacker, float64(damage), hate, true)
}

func (a *Attackable) queueAttackDesire(attacker attackable.Combatant, damage, hate float64, moveToTarget bool) {
	if attacker == nil || (a.actor.SiegeGuard() && attacker.SiegeGuard()) {
		return
	}
	_, hadMostHated := a.threats.MostHated()
	a.queueAttackDesireOnly(attacker, hate, moveToTarget)
	a.threats.AddDamage(attacker, damage, hate)
	a.thinkIfNoMostHated(hadMostHated)
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

// defaultWanderTimer is addWanderDesire's timer argument in seconds.
const defaultWanderTimer = 5

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

// AddFollowDesire queues a request to follow target with weight; an equal
// desire already queued gains the weight instead.
func (a *Attackable) AddFollowDesire(target attackable.Combatant, weight float64) {
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
func (a *Attackable) thinkIdle() { a.idleStep(true) }

// idleStep aborts every action, switches to walk stance and opens the
// no-desire point. toIdle also makes idle the current intention, as desire
// selection's idle does; the periodic cycle's own idle leaves the current
// one.
func (a *Attackable) idleStep(toIdle bool) {
	a.move.Stop()
	a.attack.Stop()
	if cast := a.CastController(); cast != nil {
		cast.Stop()
	}
	a.actor.ForceWalkStance()
	if toIdle {
		a.setCurrent(intention{kind: IntentionIdle})
	}
	a.atHookPoint(HookNoDesire)
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
// first attack desire, whoever it is aimed at: the selection's prune drops
// the desire of an attacker the actor does not know.
func (a *Attackable) thinkIfNoMostHated(hadMostHated bool) {
	if hadMostHated {
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

// SetBackToPeace clears both hate tables. The desire queue, the current and
// next intentions and movement stay as they are: queued attack desires decay
// or are pruned on later passes, and every other desire stays until its own
// rule removes it.
func (a *Attackable) SetBackToPeace() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setBackToPeaceLocked()
}

// StopAggroHate zeroes target's threat hate, drops its queued attack desire,
// and returns to peace when neither hate table still has a most-hated entry.
func (a *Attackable) StopAggroHate(target attackable.Combatant) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopAggroHateLocked(target)
}

// ReduceAllAggroHate subtracts amount from every threat entry and returns to
// peace when neither hate table still has a most-hated entry.
func (a *Attackable) ReduceAllAggroHate(amount float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reduceAllAggroHateLocked(amount)
}

// setBackToPeaceLocked ends combat memory: it clears both hate tables and
// nothing else.
func (a *Attackable) setBackToPeaceLocked() {
	a.threats.Clear()
	a.hates.Clear()
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

// TopDesireTarget is the final target of the desire desire selection last
// took up: the creature an attack, cast, flee or follow it took up is aimed
// at, nil once it took up a desire with no target. It outlives that
// desire and the event idle; the periodic cycle's empty-queue idle clears
// it.
func (a *Attackable) TopDesireTarget() attackable.Combatant {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.topDesireTarget
}

// SetTopDesireTarget replaces the top desire target until desire selection
// next takes up a desire or the periodic cycle idles.
func (a *Attackable) SetTopDesireTarget(target attackable.Combatant) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.topDesireTarget = target
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
// idle repeats its idle step and opens the no-desire point. An attack, cast
// or walk steps whether or not its desire is still queued: a walk an arrival
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
// heaviest queued desire current and steps it. Before the first periodic
// cycle it takes up nothing unless an ATTACK desire is queued. An
// out-of-control actor does neither, and neither runs while a social's hold
// or a flee holds the selection (selectionHeld).
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
// stepped a busy intention; one left idle, whether it was idle already or
// the selection's empty-queue idle idled it, runs the idle step again, so
// the no-desire point opens once more, and queues no idle follow or wander.
// Before the first periodic cycle the selection takes up nothing unless an
// ATTACK desire is queued, and the think steps the current intention.
// An out-of-control actor selects nothing and takes that think as a
// continue pass: a confused actor keeps swinging at its current target
// while a stunned one does nothing.
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

func (a *Attackable) think(mode thinkMode) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	updateTick := mode == thinkTick
	if updateTick {
		// Runs before mu is released, after the pass's selection and idle.
		defer a.cycleDecayLocked()
	}
	// Only an actor idle when the pass starts repeats its idle step on a
	// continue pass; one this pass drops to idle takes no step.
	idleAtStart := a.current.kind == IntentionIdle
	// The first-cycle promotion gate is decided before the see-creature
	// point: an attack desire queued there does not open it.
	instantRun := a.lifeTime.Load() == 0 && a.desires.hasKind(IntentionAttack)
	if updateTick {
		a.atHookPoint(HookSeeCreature)
	}
	a.passStep = a.step
	if updateTick && a.ticked {
		a.passStep = a.tickStep
		a.ticked = false
	}
	outOfControl := a.actor.OutOfControl()
	// Only desire selection's passes prune: the continue pass never reads
	// the queue.
	if mode != thinkContinue {
		a.pruneDesires(outOfControl)
	}
	held := a.selectionHeld()
	onEvent := mode == thinkEvent || mode == thinkAttackFinished
	if onEvent && !outOfControl && !held && a.idleOnEmptyQueue() {
		a.idleAfterFinishedAttack(mode, true)
		return nil
	}
	// busy is whether the actor is doing something as desire selection
	// starts. Selection never idles an intention whose desire left the
	// queue: only the empty-queue idle and a promotion change it.
	busy := a.current.kind != IntentionIdle
	notCasting := !a.castingNow()
	// canSelect is the first-cycle gate on every selection pass, on an event
	// as on the periodic cycle: nothing is taken up before the first cycle
	// has run unless an ATTACK desire was queued as the pass started. It is
	// read before the cycle counts itself.
	canSelect := a.lifeTime.Load() > 0 || instantRun
	// idleAfterLatch is the periodic cycle's empty-queue idle deferred past
	// a latched attack: the latched step runs first and the idle then
	// aborts it, as the cycle's idle does not wait for the latch.
	idleAfterLatch := false
	if updateTick {
		idled := false
		if _, ok := a.desires.Peek(); !ok && a.lifeTime.Load() > 0 && !a.castingNow() {
			switch {
			case held:
				// The cycle's idle does not wait for a held selection,
				// but only the selection switches to idle.
				a.idleStep(false)
				idled = true
			case a.hasLatch():
				idleAfterLatch = true
			case !outOfControl && busy:
				// An in-control actor that is not idle yet is idled by
				// the selection first; the cycle's own idle then runs
				// again unless the first queued a desire, so the
				// no-desire point opens twice.
				a.thinkIdle()
				if _, ok := a.desires.Peek(); !ok {
					a.idleStep(false)
				}
				idled = true
			default:
				// Only the selection switches to idle: an idle actor
				// stays so, and an out-of-control one, which selects
				// nothing, keeps its intention for when control returns.
				a.idleStep(false)
				idled = true
			}
		}
		a.lifeTime.Add(1)
		if idled {
			a.topDesireTarget = nil
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
			a.idleStep(false)
			a.topDesireTarget = nil
		}
		if mode == thinkAttackFinished && notCasting {
			return a.continueCurrent(idleAtStart)
		}
		return nil
	}
	if mode == thinkContinue {
		if !notCasting {
			return nil
		}
		return a.continueCurrent(idleAtStart)
	}
	var err error
	if notCasting && !held {
		// Before the first cycle the selection takes up nothing; a finished
		// swing's own think still steps the current intention.
		err = a.promoteAndStep(mode, canSelect)
	}
	if idleAfterLatch {
		if _, ok := a.desires.Peek(); !ok && !a.castingNow() {
			a.idleStep(false)
			a.topDesireTarget = nil
		}
	}
	a.idleAfterFinishedAttack(mode, idleAtStart)
	return err
}

// selectionHeld reports whether desire selection and its event idle wait
// this pass: a social animation's hold has not run out, or a flee is
// current with its own desire still queued. Neither holds the periodic
// cycle's idle or a continue pass.
//
// The flee's own desire is the one it was taken up from: same target, start
// and distance. A later flee from the same target, queued once the first
// one's desire is gone, starts elsewhere and does not hold the selection, so
// it can be taken up.
func (a *Attackable) selectionHeld() bool {
	if a.now().Before(a.socialHold) {
		return true
	}
	if a.current.kind != IntentionFlee {
		return false
	}
	return a.desires.anyMatch(func(d *Desire) bool {
		return d.Kind == IntentionFlee && sameCombatant(d.FinalTarget, a.current.target) &&
			d.Location == a.current.loc && d.Distance == a.current.distance
	})
}

// idleAfterFinishedAttack is the finished attack's think on an in-control
// actor, after desire selection: an actor left idle runs the idle step
// again, so the no-desire point opens.
// wasIdle reports that the actor was idle when the pass started or that the
// selection's empty-queue idle idled it.
func (a *Attackable) idleAfterFinishedAttack(mode thinkMode, wasIdle bool) {
	if mode == thinkAttackFinished && wasIdle && a.current.kind == IntentionIdle {
		a.thinkIdle()
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
		return a.thinkAttack()
	case IntentionCast:
		return a.thinkCast()
	case IntentionFollow:
		return a.thinkFollow()
	case IntentionMoveTo:
		a.thinkMoveTo()
	case IntentionMoveRoute:
		a.thinkMoveRoute()
	case IntentionPickUp:
		a.thinkPickUp()
	}
	return nil
}

// promoteAndStep promotes the next desire and runs one step of the current
// intention. A lost attack or cast target ends the step; the pass does not
// re-select. With nothing promoted (see promoteNext), or selection closed
// (canSelect false) before the first periodic cycle, the current intention
// takes no step, except on a finished swing, whose own think steps it; that
// think has no route-walk step.
func (a *Attackable) promoteAndStep(mode thinkMode, canSelect bool) error {
	promoted := canSelect && a.promoteNext()
	if !promoted && mode != thinkAttackFinished {
		return nil
	}
	switch a.current.kind {
	case IntentionAttack:
		a.lastDesire = IntentionAttack
		return a.thinkAttack()
	case IntentionCast:
		a.lastDesire = IntentionCast
		return a.thinkCast()
	case IntentionFollow:
		a.lastDesire = IntentionFollow
		return a.thinkFollow()
	case IntentionNothing:
		// Doing nothing has no step: the actor keeps still while the
		// desire outweighs the rest.
		a.lastDesire = IntentionNothing
	case IntentionFlee:
		// A flee steps once, on the pass that promotes it; while its
		// desire stays queued, the selection holds.
		if !promoted {
			return nil
		}
		a.thinkFlee()
		a.lastDesire = IntentionFlee
	case IntentionSocial:
		if !promoted {
			return nil
		}
		a.thinkSocial()
		a.lastDesire = IntentionSocial
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
	case IntentionMoveRoute:
		if !promoted {
			return nil
		}
		a.thinkMoveRoute()
		a.lastDesire = IntentionMoveRoute
	case IntentionPickUp:
		a.thinkPickUp()
		a.lastDesire = IntentionPickUp
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
	if a.lifeTime.Load() == 0 || a.current.kind == IntentionIdle || a.hasLatch() || a.castingNow() {
		return false
	}
	if _, ok := a.desires.Peek(); ok {
		return false
	}
	a.thinkIdle()
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
// and updates the latch from it. It reports whether anything was promoted:
// nothing is during the hit animation, with no promotable desire, or when a
// wander is current and the pick is a wander too.
//
// Desire selection always makes the pick current, whatever the current
// intention is, so a heavier attack on another target, a cast or a walk
// takes over a running attack. Replacing the intention with a different
// one drops any follow task and the queued next intention the old one left
// behind. Taking up any desire but a route walk takes the actor off its
// route.
func (a *Attackable) promoteNext() bool {
	if a.inHitAnimation() {
		return false
	}
	next, ok := a.nextToDo()
	if !ok {
		return false
	}
	if a.current.kind == IntentionWander && next.kind == IntentionWander {
		return false
	}
	if next.kind != IntentionMoveRoute {
		a.leaveRoute()
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
	a.topDesireTarget = next.target
	return true
}

// same reports whether o is the same intention as i: same kind, aimed at
// the same target with the same skill, or walking to the same location or
// route.
func (i intention) same(o intention) bool {
	return i.kind == o.kind && sameCombatant(i.target, o.target) && i.skill == o.skill && i.loc == o.loc && i.itemObjectID == o.itemObjectID && i.route == o.route
}

// nextToDo returns the latched attack when one is set, else the heaviest
// queued desire as an intention. ok is false when there is neither or the
// heaviest desire's kind is not promotable.
func (a *Attackable) nextToDo() (next intention, ok bool) {
	if a.hasLatch() {
		return a.latched, true
	}
	desire, ok := a.desires.Peek()
	if !ok {
		return intention{}, false
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
	case IntentionNothing:
		next = intention{kind: IntentionNothing, timer: desire.Timer}
	case IntentionFlee:
		next = intention{kind: IntentionFlee, target: desire.FinalTarget, loc: desire.Location, distance: desire.Distance}
	case IntentionSocial:
		next = intention{kind: IntentionSocial, itemObjectID: desire.ItemObjectID, timer: desire.Timer}
	case IntentionPickUp:
		next = intention{kind: IntentionPickUp, itemObjectID: desire.ItemObjectID}
	case IntentionMoveTo:
		next = intention{kind: IntentionMoveTo, loc: desire.Location}
	case IntentionMoveRoute:
		next = intention{kind: IntentionMoveRoute, route: desire.RouteName}
	default:
		return intention{}, false
	}
	return next, true
}

// Tick advances the AI clock. Every third step it marks the cycle's hate
// refresh and attack, cast, and nothing-desire weight decay due; the cycle's
// TickThink runs them after its selection. A decay a previous cycle left
// due, because no TickThink followed its Tick, runs first.
func (a *Attackable) Tick() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cycleDecayLocked()
	a.tickStep, a.ticked = a.step, true
	a.step++
	if a.step%3 != 0 {
		return
	}
	a.cycleDecayDue = true
	a.step = 0
}

// cycleDecayLocked runs the periodic hate refresh and weight decay if a Tick
// marked them due.
func (a *Attackable) cycleDecayLocked() {
	if !a.cycleDecayDue {
		return
	}
	a.cycleDecayDue = false
	a.refreshCombatMemory()
	a.reduceAllAggroHateLocked(attackHateDecay)
	a.desires.DecreaseWeightByType(IntentionAttack, attackHateDecay)
	a.desires.DecreaseWeightByType(IntentionCast, castDesireDecay)
	a.desires.DecreaseWeightByType(IntentionNothing, nothingDesireDecay)
}

// KeepTerritory makes m the actor's out-of-territory state, the one its
// earlier lives left: when the last of their arrivals was outside, the
// stale-hate sweep goes on, on its grid, on this life's queue, and the next
// arrival outside opens no out-of-territory point. Call it once, before the
// actor is published.
func (a *Attackable) KeepTerritory(m *TerritoryMemory) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.territory = m
	outside, due := m.Load()
	if !outside {
		return
	}
	next, d := NextOnGrid(due, a.now(), ootSweepPeriod)
	a.territory.Store(true, next)
	a.armOOTSweepAt(d)
}

// syncOOTSweepLocked settles the out-of-territory state on an arrival. One
// inside territory disarms the stale-hate sweep. The first one outside opens
// the out-of-territory point, then arms the sweep: 100 ms later, then every
// 10 s on a fixed grid. Nothing else disarms it; the actor's queue closing
// when it leaves the world cancels this life's firing, and the next life
// resumes it on its grid (KeepTerritory).
func (a *Attackable) syncOOTSweepLocked() {
	if a.actor.InTerritory() {
		a.territory.Store(false, time.Time{})
		a.stopOOTSweep()
		return
	}
	if outside, _ := a.territory.Load(); outside {
		return
	}
	a.atHookPoint(HookOutOfTerritory)
	a.stopOOTSweep()
	a.armOOTSweep(ootSweepInitialDelay)
}

// stopOOTSweep cancels the stale-hate sweep's pending firing, if any.
func (a *Attackable) stopOOTSweep() {
	a.ootSeq++
	if a.ootSweep != nil {
		a.ootSweep.Stop()
		a.ootSweep = nil
	}
}

// armOOTSweep schedules the next stale-hate sweep d from now on the actor's
// queue and records the actor outside with that deadline.
func (a *Attackable) armOOTSweep(d time.Duration) {
	a.territory.Store(true, a.now().Add(d))
	a.armOOTSweepAt(d)
}

// armOOTSweepAt schedules this life's next stale-hate sweep firing d from
// now on the actor's queue.
func (a *Attackable) armOOTSweepAt(d time.Duration) {
	seq := a.ootSeq
	a.ootSweep = a.actor.After(d, func() { a.sweepStaleThreats(seq) })
}

// sweepStaleThreats is one firing of the out-of-territory stale-hate sweep:
// it re-arms on the fixed-rate grid, then, while the actor stands outside
// its territory, stops the hate of each threat entry whose last damage is
// staleThreatAge old or more and drops the attack desire on it. A firing of
// a sweep since disarmed does nothing.
func (a *Attackable) sweepStaleThreats(seq uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if seq != a.ootSeq {
		return
	}
	now := a.now()
	_, due := a.territory.Load()
	_, d := NextOnGrid(due.Add(ootSweepPeriod), now, ootSweepPeriod)
	a.armOOTSweep(d)
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

// pruneDesires drops invalid cast desires, then every desire aimed at a
// creature the actor no longer knows or that is dead, then, unless the actor
// is out of control, attack desires whose target is beyond
// attackDesireRange. It only drops desires: hate, the current intention and
// the next one stay as they are. No queued kind carries a non-creature
// target: a pick-up names its item by object id only, so the unknown-object
// prune never drops one, and one whose item is gone stays queued.
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
	a.desires.RemoveIf(func(d *Desire) bool {
		return d.FinalTarget != nil && (!a.actor.Knows(d.FinalTarget) || d.FinalTarget.AlikeDead())
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

// thinkAttack advances one IntentionAttack step. A lost target (see
// targetLost) ends the step with nothing changed: the intention, hate and
// desires stay as they are, and the next desire selection decides. A dead
// target the actor still knows is not lost: the follow and attack gates
// refuse it. The return is any broadcast error from a synchronous call this
// step made.
func (a *Attackable) thinkAttack() error {
	if a.actor.DenyAIAction() {
		return nil
	}

	target := a.current.target
	if a.targetLost(target, "") {
		return nil
	}

	following, err := a.move.MaybeStartOffensiveFollow(target, a.actor.PhysicalAttackRange())
	if following {
		return err
	}

	if a.attack.BowCoolingDown() || a.attack.AttackingNow() {
		a.next = a.current
		return nil
	}

	if !a.attack.CanAttack(target) {
		return nil
	}

	a.move.Stop()
	a.attack.DoAttack(target)
	return nil
}

// thinkCast advances an IntentionCast desire once it has been promoted to
// the current intention: pre-movement validation, closing distance on the
// target, planting and facing it once the cast animation is long enough to
// warrant it, then the final cast attempt. It mirrors thinkAttack's shape
// for skill casts instead of physical attacks; a lost target ends the step
// with nothing changed, as there.
func (a *Attackable) thinkCast() error {
	cast := a.CastController()
	if a.actor.DenyAIAction() || cast == nil {
		return nil
	}
	if cast.Disabled() {
		return nil
	}

	target := a.current.target
	ref := a.current.skill
	if a.targetLost(target, cast.SkillType(ref)) {
		return nil
	}

	if !cast.CanAttempt(target, ref) {
		return nil
	}

	following, err := a.move.MaybeStartOffensiveFollow(target, cast.Range(ref))
	if following {
		a.actor.ForceRunStance()
		return err
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
		return nil
	}

	cast.Cast(target, ref)
	return nil
}

func (a *Attackable) thinkMoveTo() {
	if a.actor.DenyAIAction() {
		return
	}
	if a.actor.MovementDisabled() {
		return
	}
	// Standing on the point already still walks there: a zero-length move
	// that arrives on the next move tick.
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
		a.clearCurrentDesire()
	}
}

func (a *Attackable) clearCurrentDesire() {
	probe := &Desire{Kind: a.current.kind, Location: a.current.loc, FinalTarget: a.current.target, Skill: a.current.skill, ItemObjectID: a.current.itemObjectID, RouteName: a.current.route}
	a.desires.RemoveIf(func(d *Desire) bool { return d.Equal(probe) })
}

// thinkFlee is the flee step: the actor runs, in run stance, its distance
// straight away from where the target stands now. It does not move when it
// cannot, flees from itself, has a distance under 10, or already stands its
// distance or more from where the flee was queued.
func (a *Attackable) thinkFlee() {
	if a.actor.MovementDisabled() {
		return
	}
	target, distance := a.current.target, a.current.distance
	if target == nil || target.ObjectID() == a.actor.ObjectID() || distance < 10 {
		return
	}
	ox, oy, oz := a.actor.Position()
	from := location.Location{X: ox, Y: oy, Z: oz}
	if from.Distance2D(a.current.loc) >= float64(distance) {
		return
	}
	a.actor.ForceRunStance()
	tx, ty, _ := target.Position()
	_, _ = a.move.MoveToLocation(from.FleeFrom(tx, ty, distance))
}

// thinkSocial is the social step: the desire leaves the queue, and an actor
// still in control stops, plays the animation and holds desire selection for
// the social's timer.
func (a *Attackable) thinkSocial() {
	a.clearCurrentDesire()
	if a.actor.DenyAIAction() {
		return
	}
	a.socialHold = a.now().Add(time.Duration(a.current.timer) * time.Millisecond)
	a.move.Stop()
	a.actor.SocialAction(int(a.current.itemObjectID))
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

// refreshCombatMemory is the periodic hate-table refresh: an empty threat
// table drops every attack desire; otherwise a dead attacker's threat hate
// stops, an attacker the actor no longer knows leaves the table, and either
// loses its attack desire. The skill hate table drops both kinds of entry.
// Not knowing an attacker covers one that is off the grid and one whose
// acting player is invisible.
func (a *Attackable) refreshCombatMemory() {
	if a.threats.IsEmpty() {
		a.desires.RemoveKind(IntentionAttack)
	} else {
		for _, target := range a.threats.Refresh(a.actor.Knows) {
			a.desires.Remove(IntentionAttack, target)
		}
	}
	a.hates.Refresh(a.actor.Knows)
}

// targetLost reports whether an attack or cast step has lost target: there
// is none, it is no longer in the world, or the actor does not know it. A
// SUMMON_FRIEND cast (skillType) keeps a target the actor does not know. It
// changes nothing.
func (a *Attackable) targetLost(target attackable.Combatant, skillType string) bool {
	if target == nil || !a.actor.InWorld(target) {
		return true
	}
	if skillType == "SUMMON_FRIEND" {
		return false
	}
	return !a.actor.Knows(target)
}
