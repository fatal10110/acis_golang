package move

import (
	"errors"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	BroadcastMove(Event) error
	BroadcastStop() error
}

type pawnFollowActor interface {
	OffensiveFollowIsPawnMove() bool
}

type offensiveFollowLeadActor interface {
	OffensiveFollowLead() bool
}

type targetKnower interface {
	Knows(attackable.Combatant) bool
}

type offensiveFollowTickerOwner interface {
	OwnsOffensiveFollowTicker() bool
}

// homePathRecovery is implemented by hostile NPCs whose return-home path can
// stall on geodata and must teleport after repeated blocked resolutions.
type homePathRecovery interface {
	GeoPathFailCount() int
	ResetGeoPathFailCount()
	AddGeoPathFailCount()
	TeleportTo(location.Location)
}

const homeGeoFailLimit = 10

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
type Controller struct {
	move            *CreatureMove
	self            Actor
	positionUpdates PositionUpdateRegistry

	mu                     sync.Mutex
	offensiveTarget        attackable.Combatant
	offensiveRange         int
	offensiveFollowElapsed time.Duration
}

// NewController adapts move for self, the position/footprint of the actor
// move drives.
func NewController(move *CreatureMove, self Actor) (*Controller, error) {
	if move == nil {
		return nil, errors.New("move: nil creature move")
	}
	if self == nil {
		return nil, errors.New("move: nil self")
	}
	// A route split across geopath waypoints re-broadcasts on every segment
	// advance, not just the first: without it, clients keep predicting the
	// original straight-line walk and visibly cut through obstacles the
	// server itself routed around.
	move.SetSegmentAdvancedHook(func(event Event) error {
		// Continuations describe the next route leg, so they must carry its
		// waypoint rather than a follow target.
		event.FollowTarget = 0
		event.FollowOffset = 0
		// Reference rotates toward the new leg immediately before
		// broadcasting it (CreatureMove.java moveToNextRoutePoint,
		// setHeadingTo(destination) directly above the MoveToLocation send).
		self.SetHeading(event.Origin.HeadingTo(event.Destination))
		return self.BroadcastMove(event)
	})
	move.SetBlockedHook(func() { _ = self.BroadcastStop() })
	return &Controller{move: move, self: self}, nil
}

// ObjectID returns the actor id this controller moves.
func (c *Controller) ObjectID() int32 {
	return c.self.ObjectID()
}

// RegionActor returns the world-tracked actor this controller advances, when
// the actor participates in region activity.
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
// toward its current position) is left alone rather than re-issued.
//
// This does not reproduce the reference behavior's line-of-sight branch (an
// out-of-range NPC that also can't see its target still counts it as
// followable) — the controller has no line-of-sight input.
func (c *Controller) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maybeStartFollow(target, attackRange, FollowOffensive)
}

// MaybeStartFriendlyFollow arms a friendly follow task and starts moving
// toward target when it sits farther than offset plus both actors'
// footprints. Friendly follow broadcasts a plain movement request; follow
// identity stays server-side for the follow tick.
func (c *Controller) MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearOffensiveFollow()
	return c.maybeStartFollow(target, offset, FollowFriendly)
}

func (c *Controller) maybeStartFollow(target attackable.Combatant, offset int, mode FollowMode) (bool, error) {
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
	if mode == FollowOffensive && c.selfHasOffensiveFollowLead() && targetMoving(target) {
		totalRadius += 50
	}
	inRange := in2DRange(origin, dest, totalRadius)
	if mode == FollowOffensive {
		if actor, ok := c.self.(pawnFollowActor); ok && actor.OffensiveFollowIsPawnMove() {
			inRange = location.In3DRange(origin.X, origin.Y, origin.Z, dest.X, dest.Y, dest.Z, totalRadius)
		}
	}
	if inRange {
		if mode == FollowFriendly {
			c.move.StartFriendlyFollow(target.ObjectID(), offset)
		} else {
			c.clearOffensiveFollow()
		}
		return false, nil
	}

	switch mode {
	case FollowFriendly:
		c.move.StartFriendlyFollow(target.ObjectID(), offset)
	case FollowOffensive:
		c.move.StartOffensiveFollow(target.ObjectID(), offset)
		if !c.selfOwnsOffensiveFollowTicker() {
			c.offensiveTarget = target
			c.offensiveRange = offset
		}
	default:
		return false, nil
	}
	if !c.move.Moving() || c.move.Destination() != dest {
		event, err := c.move.MoveToLocation(dest)
		if err != nil {
			// Can't actually approach (for example, zero speed): don't
			// report "still moving" — that would strand the caller waiting
			// on progress that will never happen.
			c.clearOffensiveFollow()
			return false, nil
		}
		if mode == FollowOffensive {
			if actor, ok := c.self.(pawnFollowActor); ok && actor.OffensiveFollowIsPawnMove() {
				event.FollowTarget = target.ObjectID()
				event.FollowOffset = offset
			}
		}
		broadcastErr := c.self.BroadcastMove(event)
		c.addPositionUpdate()
		return true, broadcastErr
	}
	return true, nil
}

// MoveHome requests movement toward home, broadcasts the move, and
// registers for correction ticks the same way any other movement request
// does — otherwise this controller's world presence would stay at the
// stale pre-move cell for the entire walk back. When geodata cannot resolve
// a route, failed attempts accumulate; after homeGeoFailLimit blocked
// resolutions the actor teleports to home instead of retrying silently.
func (c *Controller) MoveHome(home location.Location) error {
	recovery, hasRecovery := c.self.(homePathRecovery)
	if hasRecovery && recovery.GeoPathFailCount() >= homeGeoFailLimit {
		c.move.CancelMove()
		recovery.TeleportTo(home)
		recovery.ResetGeoPathFailCount()
		c.removePositionUpdate()
		return nil
	}

	event, outcome, err := c.move.MoveToLocationWithPathOutcome(home)
	if err != nil {
		if hasRecovery {
			recovery.AddGeoPathFailCount()
		}
		return err
	}
	if hasRecovery {
		switch outcome {
		case pathRouted:
			recovery.ResetGeoPathFailCount()
		case pathFailed:
			recovery.AddGeoPathFailCount()
		}
	}
	broadcastErr := c.self.BroadcastMove(event)
	c.addPositionUpdate()
	return broadcastErr
}

// MoveToLocation starts a direct movement request and reports whether it was
// accepted.
func (c *Controller) MoveToLocation(target location.Location) (bool, error) {
	event, err := c.move.MoveToLocation(target)
	if err != nil {
		return false, nil
	}
	broadcastErr := c.self.BroadcastMove(event)
	c.addPositionUpdate()
	return true, broadcastErr
}

// MoveToLocationEvent behaves like MoveToLocation but also returns the
// accepted move's Event, for callers that need the move detail alongside
// acceptance (task.Walker's WalkerActor contract).
func (c *Controller) MoveToLocationEvent(target location.Location) (Event, error) {
	event, err := c.move.MoveToLocation(target)
	if err != nil {
		return Event{}, err
	}
	broadcastErr := c.self.BroadcastMove(event)
	c.addPositionUpdate()
	return event, broadcastErr
}

// Stop cancels any active follow task and any movement already under way,
// broadcasting a stop-in-place packet when there was movement to cancel —
// otherwise a client that already received the move request keeps walking
// toward the stale destination until it separately resyncs.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	wasMoving := c.move.Moving() || c.move.Following()
	c.clearOffensiveFollow()
	c.move.CancelMove()
	c.removePositionUpdate()
	if wasMoving {
		return c.self.BroadcastStop()
	}
	return nil
}

// SetArrived records the callback invoked once movement this controller
// started reaches its destination. A nil callback (the default) makes
// arrival a no-op.
func (c *Controller) SetArrived(arrived func()) {
	c.move.SetArrivedHook(func() {
		c.mu.Lock()
		following := c.offensiveTarget != nil
		c.mu.Unlock()
		if !following {
			c.removePositionUpdate()
		}
		if arrived != nil {
			arrived()
		}
	})
}

// PositionUpdate advances one movement correction tick, syncing this
// controller's world presence to the newly interpolated position. An
// ordinary interpolation tick does not itself rebroadcast a movement
// packet — resending one every tick would restart the client-side walk
// animation instead of just correcting server-side state — but crossing a
// geopath segment boundary inside UpdatePosition does rebroadcast (via the
// segment-advanced hook installed in NewController), deliberately, so the
// client restarts its per-leg animation the same way the reference client
// does on each routed waypoint. It returns false once the move has
// stopped.
//
// Reaching the destination fires the arrived hook synchronously inside
// UpdatePosition, before this returns — including SetArrived's own
// removePositionUpdate call. If that hook (an NPC's AI, say) starts a new
// move as a result, c.move is moving again by the time UpdatePosition
// returns, so the fresh state here — not the stale result of this tick —
// decides whether to unregister.
func (c *Controller) PositionUpdate() bool {
	event, moving := c.move.UpdatePosition(PositionUpdateInterval)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recheckOffensiveFollow()
	if !moving {
		if !c.move.Moving() && c.offensiveTarget == nil {
			c.removePositionUpdate()
		}
		return c.move.Moving() || c.offensiveTarget != nil
	}
	c.self.SyncPosition(event.Origin)
	return true
}

func (c *Controller) recheckOffensiveFollow() {
	if c.offensiveTarget == nil {
		return
	}
	if actor, ok := c.self.(targetKnower); ok && !actor.Knows(c.offensiveTarget) {
		c.clearOffensiveFollow()
		return
	}
	c.offensiveFollowElapsed += PositionUpdateInterval
	if c.offensiveFollowElapsed < c.move.FollowInterval() {
		return
	}
	c.offensiveFollowElapsed = 0
	_, _ = c.maybeStartFollow(c.offensiveTarget, c.offensiveRange, FollowOffensive)
}

func (c *Controller) selfOwnsOffensiveFollowTicker() bool {
	actor, ok := c.self.(offensiveFollowTickerOwner)
	return ok && actor.OwnsOffensiveFollowTicker()
}

func (c *Controller) selfHasOffensiveFollowLead() bool {
	actor, ok := c.self.(offensiveFollowLeadActor)
	return ok && actor.OffensiveFollowLead()
}

func targetMoving(target attackable.Combatant) bool {
	if target, ok := target.(interface{ IsMoving() bool }); ok {
		return target.IsMoving()
	}
	if target, ok := target.(interface{ Move() *CreatureMove }); ok {
		return target.Move().Moving()
	}
	return false
}

func (c *Controller) clearOffensiveFollow() {
	c.move.CancelFollow()
	c.offensiveTarget = nil
	c.offensiveRange = 0
	c.offensiveFollowElapsed = 0
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
