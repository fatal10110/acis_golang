package move

import (
	"errors"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Located is the position and footprint of a live actor a Controller reads
// to resolve follow/attack ranges: the actor it drives (self), and any
// combat target it is asked to close distance on.
type Located interface {
	Position() (x, y, z int)
	CollisionRadius() float64
}

// Actor is the actor a Controller drives (self): its position/footprint,
// plus its ability to broadcast its own movement to the world.
type Actor interface {
	Located
	ObjectID() int32
	SyncPosition(location.Location)
	SetHeading(int)
	BroadcastMove(event.Move)
	BroadcastStop()
	// OwnsOffensiveFollowTicker reports that the actor's own AI already
	// rechecks an offensive follow, so the controller must not track it.
	OwnsOffensiveFollowTicker() bool
	// MovementDisabled reports that the actor cannot start moving (rooted,
	// immobilized, stunned, asleep, ...).
	MovementDisabled() bool
}

type pawnFollowActor interface {
	OffensiveFollowIsPawnMove() bool
}

type offensiveFollowLeadActor interface {
	OffensiveFollowLead() bool
}

// npcOffensiveFollowActor is an NPC's extra offensive follow input: its AI's
// current intention decides whether it may close in at all, and a target in
// reach but out of its sight is still approached.
type npcOffensiveFollowActor interface {
	IntentionMovesToTarget() bool
	CanSee(attackable.Combatant) bool
}

type targetKnower interface {
	Knows(attackable.Combatant) bool
}

// homePathRecovery is implemented by hostile NPCs whose return-home path can
// stall on geodata and must teleport after repeated blocked resolutions.
// TeleportTo owns aborting the walk in progress.
type homePathRecovery interface {
	GeoPathFailCount() int
	ResetGeoPathFailCount()
	AddGeoPathFailCount()
	TeleportTo(location.Location)
}

// HomeGeoFailLimit is the consecutive blocked return-home path count at
// which MoveHome teleports to spawn instead of walking.
const HomeGeoFailLimit = 10

// PositionUpdater is the moving actor surface consumed by the position
// update task. PositionUpdate must deregister itself from whatever
// PositionUpdateRegistry it was added through once it no longer needs
// ticks — a false return tells the task's own tick loop only that this
// actor needs no further action this round, not that the task should
// remove it: by the time PositionUpdate returns, a concurrent goroutine
// may already have re-registered the same actor for a new move.
type PositionUpdater interface {
	ObjectID() int32
	PositionUpdate() bool
	// Queue is the queue position updates run on.
	Queue() *sim.Queue
}

// PositionUpdateRegistry tracks actors that need position-update ticks.
type PositionUpdateRegistry interface {
	Add(PositionUpdater)
	Remove(PositionUpdater)
}

// Controller adapts one CreatureMove to the hostile NPC AI loop's expected
// movement surface, translating a follow/attack-range decision into
// CreatureMove's StartOffensiveFollow/CancelFollow calls and a return-home
// request into MoveToLocation.
//
// mu guards the follow state below. Another actor's effect stops the move
// synchronously from its own queue (AbortAll or StopMove).
type Controller struct {
	move            *CreatureMove
	self            Actor
	sink            event.Sink
	positionUpdates PositionUpdateRegistry

	mu                     sync.Mutex
	offensiveTarget        attackable.Combatant
	offensiveRange         int
	offensiveFollowElapsed time.Duration
	// friendlyTarget is a player's friendly follow, which this controller
	// rechecks on the position-update ticks.
	friendlyTarget        attackable.Combatant
	friendlyOffset        int
	friendlyFollowElapsed time.Duration
}

// NewController adapts move for self, the position/footprint of the actor
// move drives. sink receives Arrived and MoveBlocked; nil drops Arrived and
// answers a blocked move with the stopped-cell correction itself.
func NewController(move *CreatureMove, self Actor, sink event.Sink) (*Controller, error) {
	if move == nil {
		return nil, errors.New("move: nil creature move")
	}
	if self == nil {
		return nil, errors.New("move: nil self")
	}
	c := &Controller{move: move, self: self, sink: sink}
	move.setOwner(c)
	return c, nil
}

// segmentAdvanced re-broadcasts a route split across geopath waypoints on
// every segment advance, not just the first: without it, clients keep
// predicting the original straight-line walk and visibly cut through
// obstacles the server itself routed around.
func (c *Controller) segmentAdvanced(ev event.Move) {
	// Continuations describe the next route leg, so they must carry its
	// waypoint rather than a follow target.
	ev.FollowTarget = 0
	ev.FollowOffset = 0
	c.broadcastMoveStart(ev)
}

// broadcastMoveStart turns the actor toward the leg ev starts, then shows
// observers the walk. Every walk start and route leg faces its destination
// before the movement is broadcast, so a later stop or appearance carries the
// walk's direction; a same-cell request faces heading 0.
func (c *Controller) broadcastMoveStart(ev event.Move) {
	c.self.SetHeading(ev.Origin.HeadingTo(ev.Destination))
	c.self.BroadcastMove(ev)
}

// blocked reports an in-flight move stopped by a newly blocked geodata path.
// The sink owes observers BroadcastBlockedCorrection, ordered around its own
// reaction; with no sink the correction is sent here.
//
// A player's attack approach ends with it: the player stays where the walk
// stopped until its attack thinks again.
func (c *Controller) blocked() {
	if c.selfFollowsByPawn() {
		c.mu.Lock()
		if c.move.FollowMode() == FollowOffensive {
			c.clearFollow()
		}
		c.mu.Unlock()
	}
	if c.sink == nil {
		c.BroadcastBlockedCorrection()
		return
	}
	c.sink.Emit(event.MoveBlocked{})
}

// arrived unregisters position ticks for a move that is not chasing a
// target, then reports the arrival.
func (c *Controller) arrived() {
	c.mu.Lock()
	following := c.tracksFollowLocked()
	c.mu.Unlock()
	if !following {
		c.removePositionUpdate()
	}
	if c.sink != nil {
		c.sink.Emit(event.Arrived{})
	}
}

// ObjectID returns the actor id this controller moves.
func (c *Controller) ObjectID() int32 {
	return c.self.ObjectID()
}

// RegionActor returns the world-tracked actor this controller advances, when
// the actor participates in region activity.
// Hostile movement drives a non-world forwarding ref, which reports nil.
func (c *Controller) RegionActor() world.Tracked {
	tracked, _ := c.self.(world.Tracked)
	return tracked
}

// SetPositionUpdates records the registry that should tick this controller
// while movement is in flight.
func (c *Controller) SetPositionUpdates(updates PositionUpdateRegistry) {
	if c.positionUpdates != nil && c.move.Moving() {
		c.positionUpdates.Remove(c)
	}
	c.positionUpdates = updates
	if c.positionUpdates != nil && c.move.Moving() {
		c.positionUpdates.Add(c)
	}
}

// MaybeStartOffensiveFollow starts or refreshes a follow task toward target
// when it sits farther than attackRange plus both actors' footprints,
// issues the movement request to actually close the distance, and reports
// whether the caller should wait for that movement instead of attacking
// now. A target with no known position/footprint can't be followed and
// reports false. A target already converged on (movement already under way
// toward its current position) is left alone rather than re-issued, except
// by a player free to move: every think of a player's attack re-sends its
// pawn walk toward the target from where the player stands now.
//
// An actor that cannot move, or an NPC whose current intention holds its
// ground, never starts a follow: out of range it still reports true, so the
// caller waits instead of attacking, but no movement is issued. A follow
// already running toward target keeps running; the effects that lock
// movement stop it themselves.
//
// An NPC free to close in that has target within reach but cannot see it
// follows it anyway, down to the target's own footprint, and reports true so
// it does not swing or cast through the obstacle.
func (c *Controller) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	return c.maybeStartOffensiveFollow(target, attackRange, false)
}

// RecheckOffensiveFollow is one run of an offensive follow task the actor's
// own AI ticks (see Actor.OwnsOffensiveFollowTicker): MaybeStartOffensiveFollow,
// except that a swimming or flying actor measures whether target is already
// in reach in 3D.
func (c *Controller) RecheckOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	return c.maybeStartOffensiveFollow(target, attackRange, true)
}

func (c *Controller) maybeStartOffensiveFollow(target attackable.Combatant, attackRange int, tick bool) (bool, error) {
	// Read before mu: MovementDisabled reads the actor's effect state, and
	// effect hooks stop this controller (taking mu) from other queues.
	disabled := c.self.MovementDisabled()
	npcActor, isNPC := c.self.(npcOffensiveFollowActor)
	holds := isNPC && !npcActor.IntentionMovesToTarget()
	c.mu.Lock()
	defer c.mu.Unlock()
	blocked := (disabled || holds) && !c.followingLocked(target, FollowOffensive)
	var unseen func() bool
	if isNPC && !disabled && !holds {
		unseen = func() bool { return !npcActor.CanSee(target) }
	}
	reissue := !disabled && c.selfFollowsByPawn()
	return c.maybeStartFollow(target, attackRange, FollowOffensive, tick, blocked, unseen, reissue)
}

// HoldOffensiveFollow is MaybeStartOffensiveFollow for an intention that
// may not walk (a shift-held attack): it reports whether target sits out of
// attackRange plus both actors' footprints, and never starts a follow or a
// movement toward it. A target within reach drops any follow task, as
// MaybeStartOffensiveFollow does.
func (c *Controller) HoldOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Blocked, the follow returns before any follow, movement or error.
	outOfRange, _ := c.maybeStartFollow(target, attackRange, FollowOffensive, false, true, nil, false)
	return outOfRange
}

// MaybeStartFriendlyFollow arms a friendly follow task and starts moving
// toward target when it sits farther than offset plus both actors'
// footprints. Friendly follow broadcasts a plain movement request; follow
// identity stays server-side for the follow tick. An actor that cannot move
// starts nothing and reports false, leaving any running follow untouched; a
// friendly follow already running toward target keeps running.
//
// A player follows differently (see startPawnFriendlyFollow) and always
// reports true once the follow is armed.
func (c *Controller) MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error) {
	disabled := c.self.MovementDisabled()
	c.mu.Lock()
	defer c.mu.Unlock()
	if disabled && !c.followingLocked(target, FollowFriendly) {
		return false, nil
	}
	c.clearFollow()
	if c.selfFollowsByPawn() {
		c.startPawnFriendlyFollow(target, offset)
		return true, nil
	}
	return c.maybeStartFollow(target, offset, FollowFriendly, false, false, nil, false)
}

// CancelFriendlyFollow drops a player's friendly follow task, leaving a walk
// already under way running to its destination. Any other follow is left
// alone.
func (c *Controller) CancelFriendlyFollow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.friendlyTarget != nil {
		c.clearFollow()
	}
}

// FriendlyFollowTarget returns the target a player's friendly follow task
// follows, or nil when none runs.
func (c *Controller) FriendlyFollowTarget() attackable.Combatant {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.friendlyTarget
}

// startPawnFriendlyFollow arms a player's friendly follow task toward
// target and runs its first tick at once. The task then ticks every
// FollowInterval on the position updates: it walks toward target only when
// the player stands still farther than offset from it (no footprints), with
// the target-relative movement packet, and stops the player when target is
// no longer known.
func (c *Controller) startPawnFriendlyFollow(target attackable.Combatant, offset int) {
	c.move.StartFriendlyFollow(target.ObjectID(), offset)
	c.friendlyTarget = target
	c.friendlyOffset = offset
	c.friendlyFollowElapsed = 0
	c.addPositionUpdate()
	c.pawnFriendlyFollowTick()
}

// pawnFriendlyFollowTick is one run of a player's friendly follow task.
func (c *Controller) pawnFriendlyFollowTick() {
	target := c.friendlyTarget
	if actor, ok := c.self.(targetKnower); ok && !actor.Knows(target) {
		// The follower goes idle: the task and any walk stop.
		c.stopLocked()
		return
	}
	if c.move.Moving() {
		return
	}
	other, ok := target.(Located)
	if !ok {
		return
	}
	sx, sy, sz := c.self.Position()
	tx, ty, tz := other.Position()
	dest := location.Location{X: tx, Y: ty, Z: tz}
	if inRadius(c.move.MoveType(), location.Location{X: sx, Y: sy, Z: sz}, dest, c.friendlyOffset) {
		return
	}
	ev, outcome, err := c.move.MoveToPawnWithPathOutcome(target, c.friendlyOffset)
	if err != nil {
		return
	}
	c.applyPathFindOutcome(outcome)
	ev.FollowTarget = target.ObjectID()
	ev.FollowOffset = c.friendlyOffset
	c.broadcastMoveStart(ev)
	c.addPositionUpdate()
}

func (c *Controller) recheckFriendlyFollow() {
	if c.friendlyTarget == nil {
		return
	}
	c.friendlyFollowElapsed += PositionUpdateInterval
	if c.friendlyFollowElapsed < c.move.FollowInterval() {
		return
	}
	c.friendlyFollowElapsed = 0
	c.pawnFriendlyFollowTick()
}

// selfFollowsByPawn reports whether the actor approaches targets with the
// target-relative movement packet, as a player does.
func (c *Controller) selfFollowsByPawn() bool {
	actor, ok := c.self.(pawnFollowActor)
	return ok && actor.OffensiveFollowIsPawnMove()
}

// tracksFollowLocked reports whether this controller rechecks a follow task
// on the position-update ticks, and so must keep receiving them.
func (c *Controller) tracksFollowLocked() bool {
	return c.offensiveTarget != nil || c.friendlyTarget != nil
}

// followingLocked reports whether a follow of mode toward target is already
// running.
func (c *Controller) followingLocked(target attackable.Combatant, mode FollowMode) bool {
	return target != nil && c.move.FollowMode() == mode && c.move.FollowTarget() == target.ObjectID()
}

// maybeStartFollow resolves one follow request. tick marks a run of the
// offensive follow task rather than its start; a friendly follow request and
// a follow task run measure whether target is in reach in 3D while the actor
// swims or flies. blocked means the actor
// cannot move and has no follow toward target running: an out-of-range
// target then reports true without starting a follow or a move. unseen,
// when set, reports that an offensive target already in reach is out of
// sight, so the actor still closes in to the target's footprint. reissue
// re-sends the walk even when one toward the target's position is under way.
func (c *Controller) maybeStartFollow(target attackable.Combatant, offset int, mode FollowMode, tick, blocked bool, unseen func() bool, reissue bool) (bool, error) {
	if offset < 0 {
		return false, nil
	}

	other, ok := target.(Located)
	if !ok {
		return false, nil
	}

	sx, sy, sz := c.self.Position()
	tx, ty, tz := other.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	dest := location.Location{X: tx, Y: ty, Z: tz}

	totalRadius := followRange(offset, c.self.CollisionRadius(), other.CollisionRadius())
	if mode == FollowOffensive && c.selfHasOffensiveFollowLead() && target.IsMoving() {
		totalRadius += 50
	}
	var inRange bool
	switch {
	case mode == FollowOffensive && c.selfFollowsByPawn():
		inRange = origin.In3DRadius(dest, totalRadius)
	case mode == FollowFriendly || tick:
		inRange = inRadius(c.move.MoveType(), origin, dest, totalRadius)
	default:
		inRange = origin.In2DRadius(dest, totalRadius)
	}
	if inRange {
		if mode == FollowFriendly {
			c.move.StartFriendlyFollow(target.ObjectID(), offset)
			return false, nil
		}
		if unseen == nil || !unseen() {
			c.clearFollow()
			return false, nil
		}
		offset = int(other.CollisionRadius())
		if origin.In2DRadius(dest, followRange(offset, c.self.CollisionRadius(), other.CollisionRadius())) {
			// Already pressed against the target: the follow is armed but
			// has nowhere to walk.
			c.startOffensiveFollow(target, offset)
			return true, nil
		}
	} else if blocked {
		return true, nil
	}

	switch mode {
	case FollowFriendly:
		c.move.StartFriendlyFollow(target.ObjectID(), offset)
	case FollowOffensive:
		c.startOffensiveFollow(target, offset)
	default:
		return false, nil
	}
	if reissue || !c.move.Moving() || c.move.Destination() != dest {
		pawnMove := mode == FollowOffensive && c.selfFollowsByPawn()
		var (
			ev      event.Move
			outcome pathFindResult
			err     error
		)
		before := c.move.Position()
		switch {
		case pawnMove:
			ev, outcome, err = c.move.MoveToPawnWithPathOutcome(target, offset)
		case mode == FollowOffensive:
			ev, outcome, err = c.move.ChasePawnWithPathOutcome(target, offset)
		default:
			ev, outcome, err = c.move.MoveToLocationWithPathOutcome(dest)
		}
		c.syncRetarget(before)
		if err != nil {
			// Can't actually approach (for example, zero speed): don't
			// report "still moving" — that would strand the caller waiting
			// on progress that will never happen.
			c.clearFollow()
			return false, nil
		}
		c.applyPathFindOutcome(outcome)
		if pawnMove {
			ev.FollowTarget = target.ObjectID()
			ev.FollowOffset = offset
		}
		c.broadcastMoveStart(ev)
		c.addPositionUpdate()
		return true, nil
	}
	return true, nil
}

// MoveHome requests movement toward home, broadcasts the move, and
// registers for correction ticks the same way any other movement request
// does — otherwise this controller's world presence would stay at the
// stale pre-move cell for the entire walk back. When geodata cannot resolve
// a route, failed attempts accumulate; after HomeGeoFailLimit blocked
// resolutions the actor teleports to home instead of retrying silently.
func (c *Controller) MoveHome(home location.Location) error {
	recovery, hasRecovery := c.self.(homePathRecovery)
	if hasRecovery && recovery.GeoPathFailCount() >= HomeGeoFailLimit {
		recovery.TeleportTo(home)
		recovery.ResetGeoPathFailCount()
		return nil
	}

	before := c.move.Position()
	ev, outcome, err := c.move.MoveToLocationWithPathOutcome(home)
	c.syncRetarget(before)
	if err != nil {
		return err
	}
	c.applyPathFindOutcome(outcome)
	c.broadcastMoveStart(ev)
	c.addPositionUpdate()
	return nil
}

// MoveToLocation starts a pathfinding movement request and reports whether
// it was accepted. A blocked route still walks toward the last reachable
// point and counts as a geo-path failure for actors that recover from
// repeated stalls.
func (c *Controller) MoveToLocation(target location.Location) (bool, error) {
	before := c.move.Position()
	ev, outcome, err := c.move.MoveToLocationWithPathOutcome(target)
	c.syncRetarget(before)
	if err != nil {
		return false, nil
	}
	c.applyPathFindOutcome(outcome)
	c.broadcastMoveStart(ev)
	c.addPositionUpdate()
	return true, nil
}

// MoveToPawn starts a pawn walk toward target and broadcasts it as an
// approach that stops offset short of target, the same movement request a
// player's attack approach sends. The walk itself tracks target: it re-aims
// at target's current position on every position update of its last leg,
// ends once within offset of it, and ends where it stands once the actor no
// longer knows target. It reports whether the walk was accepted. It arms no
// follow task and drops any follow task already running, offensive or
// friendly, so no follow recheck steers the walk back toward an earlier
// target.
func (c *Controller) MoveToPawn(target Pawn, offset int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearFollow()
	before := c.move.Position()
	ev, outcome, err := c.move.MoveToPawnWithPathOutcome(target, offset)
	c.syncRetarget(before)
	if err != nil {
		return false
	}
	c.applyPathFindOutcome(outcome)
	ev.FollowTarget = target.ObjectID()
	ev.FollowOffset = offset
	c.broadcastMoveStart(ev)
	c.addPositionUpdate()
	return true
}

// MoveToLocationEvent behaves like MoveToLocation but also returns the
// accepted move's Event, for callers that need the move detail alongside
// acceptance (task.Walker's WalkerActor contract).
func (c *Controller) MoveToLocationEvent(target location.Location) (event.Move, error) {
	before := c.move.Position()
	ev, outcome, err := c.move.MoveToLocationWithPathOutcome(target)
	c.syncRetarget(before)
	if err != nil {
		return event.Move{}, err
	}
	c.applyPathFindOutcome(outcome)
	c.broadcastMoveStart(ev)
	c.addPositionUpdate()
	return ev, nil
}

// syncRetarget moves the actor's world presence to the cell a player's walk
// in flight advanced to before a move request retargeted it, if it moved.
func (c *Controller) syncRetarget(before location.Location) {
	if at := c.move.Position(); at != before {
		c.self.SyncPosition(at)
	}
}

func (c *Controller) applyPathFindOutcome(outcome pathFindResult) {
	recovery, ok := c.self.(homePathRecovery)
	if !ok {
		return
	}
	switch outcome {
	case pathRouted:
		recovery.ResetGeoPathFailCount()
	case pathFailed:
		recovery.AddGeoPathFailCount()
	}
}

// Stop cancels any active follow task and any movement already under way,
// broadcasting a stop-in-place packet when there was movement to cancel —
// otherwise a client that already received the move request keeps walking
// toward the stale destination until it separately resyncs.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func (c *Controller) stopLocked() {
	wasMoving := c.move.Moving() || c.move.Following()
	c.clearFollow()
	c.move.CancelMove()
	c.removePositionUpdate()
	if wasMoving {
		c.self.BroadcastStop()
	}
}

// CancelFollow drops any follow task, offensive or friendly, and leaves a
// walk already under way running to its destination.
func (c *Controller) CancelFollow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearFollow()
}

// CanMoveTo reports whether a straight-line geodata walk from the actor's
// current origin reaches target.
func (c *Controller) CanMoveTo(target location.Location) bool {
	return c.move.CanMoveTo(target)
}

// BroadcastBlockedCorrection snaps observers to the cell the actor actually
// stopped on. A same-cell MoveToLocation is the correction packet; StopMove
// would freeze client prediction at the stale destination.
func (c *Controller) BroadcastBlockedCorrection() {
	pos := c.move.Position()
	c.self.BroadcastMove(event.Move{Origin: pos, Destination: pos})
}

// Queue returns the queue the moving actor's work runs on.
func (c *Controller) Queue() *sim.Queue {
	return c.move.Queue()
}

// PositionUpdate advances one movement correction tick, syncing this
// controller's world presence to the newly interpolated position. An
// ordinary interpolation tick does not itself rebroadcast a movement
// packet — resending one every tick would restart the client-side walk
// animation instead of just correcting server-side state — but crossing a
// geopath segment boundary inside UpdatePosition does rebroadcast (via
// segmentAdvanced), deliberately, so the
// client restarts its per-leg animation on each routed waypoint. It returns
// false once the move has
// stopped.
//
// Reaching the destination fires the arrived hook synchronously, before
// this returns — including the controller's own removePositionUpdate call.
// If that hook (an NPC's AI, say) starts a new move as a result, c.move is
// moving again by the time the hook returns, so the fresh state here — not
// the stale result of this tick — decides whether to unregister.
//
// A pawn walk whose pawn the actor no longer knows does not step: it heads
// for its next geopath leg, or with none left ends where the actor stands,
// as an arrival. While a tracking pawn walk goes on, each step it takes
// turns the actor toward the cell it steps to, the step that ends the walk
// included, before the walk's milestone runs.
//
// A player's update walks the queue-clock time since its last update (see
// CreatureMove); any other mover's walks PositionUpdateInterval.
func (c *Controller) PositionUpdate() bool {
	var (
		u         positionUpdate
		abandoned bool
	)
	if pawn, gen := c.move.walkingPawn(); pawn != nil && !c.knowsPawn(pawn) {
		u, abandoned = c.move.abandonPawnWalk(gen)
	}
	if !abandoned {
		u = c.move.updatePosition(PositionUpdateInterval, true)
	}
	if u.pawnStep {
		c.self.SetHeading(u.from.HeadingTo(u.to))
	}
	if u.hook != nil {
		u.hook()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recheckOffensiveFollow()
	c.recheckFriendlyFollow()
	if !u.moving {
		if !c.move.Moving() && !c.tracksFollowLocked() {
			c.removePositionUpdate()
		}
		return c.move.Moving() || c.tracksFollowLocked()
	}
	c.self.SyncPosition(u.ev.Origin)
	return true
}

// knowsPawn reports whether the actor still knows pawn: a combatant through
// the actor's own known list, any other world object (a door) by the world
// grid's. A pawn that is not a world object, or an actor with no known list,
// counts as known; a world-object pawn that left the grid (a despawned door)
// does not, so the walk ends as an arrival.
func (c *Controller) knowsPawn(pawn Pawn) bool {
	if target, ok := pawn.(attackable.Combatant); ok {
		actor, ok := c.self.(targetKnower)
		return !ok || actor.Knows(target)
	}
	self, ok := c.self.(world.Tracked)
	if !ok {
		return true
	}
	target, ok := pawn.(world.Tracked)
	return !ok || world.Knows(self, target)
}

func (c *Controller) recheckOffensiveFollow() {
	if c.offensiveTarget == nil {
		return
	}
	// The follow task skips a target the actor does not know, and goes on
	// once it is known again.
	if actor, ok := c.self.(targetKnower); ok && !actor.Knows(c.offensiveTarget) {
		return
	}
	c.offensiveFollowElapsed += PositionUpdateInterval
	if c.offensiveFollowElapsed < c.move.FollowInterval() {
		return
	}
	c.offensiveFollowElapsed = 0
	_, _ = c.maybeStartFollow(c.offensiveTarget, c.offensiveRange, FollowOffensive, true, false, nil, false)
}

// startOffensiveFollow arms the offensive follow task toward target at
// offset, tracking it for rechecks unless the actor's own AI owns them. A
// player's approach runs no recheck: its pawn walk tracks the target, and
// only the attack's own next think walks it again.
func (c *Controller) startOffensiveFollow(target attackable.Combatant, offset int) {
	c.friendlyTarget = nil
	c.move.StartOffensiveFollow(target.ObjectID(), offset)
	if !c.selfOwnsOffensiveFollowTicker() && !c.selfFollowsByPawn() {
		c.offensiveTarget = target
		c.offensiveRange = offset
	}
}

func (c *Controller) selfOwnsOffensiveFollowTicker() bool {
	return c.self != nil && c.self.OwnsOffensiveFollowTicker()
}

func (c *Controller) selfHasOffensiveFollowLead() bool {
	actor, ok := c.self.(offensiveFollowLeadActor)
	return ok && actor.OffensiveFollowLead()
}

// clearFollow drops any follow task, offensive or friendly.
func (c *Controller) clearFollow() {
	c.move.CancelFollow()
	c.offensiveTarget = nil
	c.offensiveRange = 0
	c.offensiveFollowElapsed = 0
	c.friendlyTarget = nil
	c.friendlyOffset = 0
	c.friendlyFollowElapsed = 0
}

func (c *Controller) addPositionUpdate() {
	if c.positionUpdates != nil {
		c.positionUpdates.Add(c)
	}
}

func (c *Controller) removePositionUpdate() {
	if c.positionUpdates != nil {
		c.positionUpdates.Remove(c)
	}
}

// Position returns the actor's current server-authoritative position as
// tracked by the wrapped CreatureMove. An arrived hook reads this to learn
// where movement actually left the actor.
func (c *Controller) Position() location.Location {
	return c.move.Position()
}

// SetPosition reseeds the wrapped CreatureMove's position. Call it whenever
// the actor's position changes outside this controller — a client-reported
// walk, a teleport — so the next chase computes its route/duration from
// where the actor actually is, not a stale seed.
func (c *Controller) SetPosition(position location.Location) {
	c.move.SetPosition(position)
}
