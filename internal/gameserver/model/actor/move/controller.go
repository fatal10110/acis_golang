package move

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	BroadcastMove(Event)
	BroadcastStop()
}

// PositionUpdater is the moving actor surface consumed by the position
// update task.
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
//
// Controller holds no mutable state of its own — self and move are set once
// at construction — so it needs no lock; every mutation happens inside the
// wrapped CreatureMove, which is the caller's own synchronization
// responsibility per its doc comment.
type Controller struct {
	move            *CreatureMove
	self            Actor
	positionUpdates PositionUpdateRegistry
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
	return &Controller{move: move, self: self}, nil
}

// ObjectID returns the actor id this controller moves.
func (c *Controller) ObjectID() int32 {
	return c.self.ObjectID()
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
// followable) — no geodata query is wired into a live actor yet. It also
// does not re-track a target that keeps moving during the approach: this
// starts one movement request toward the target's position at call time,
// which is enough to converge on a stationary target and is re-issued
// naturally the next time the caller re-evaluates (on arrival, or on the
// next attack attempt).
func (c *Controller) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	if attackRange < 0 {
		return false
	}

	other, ok := target.(Located)
	if !ok {
		return false
	}

	sx, sy, sz := c.self.Position()
	tx, ty, tz := other.Position()
	origin := location.Location{X: sx, Y: sy, Z: sz}
	dest := location.Location{X: tx, Y: ty, Z: tz}

	totalRadius := attackRange + int(c.self.CollisionRadius()) + int(other.CollisionRadius())
	if in2DRange(origin, dest, totalRadius) {
		c.move.CancelFollow()
		return false
	}

	c.move.StartOffensiveFollow(target.ObjectID(), attackRange)
	if !c.move.Moving() || c.move.Destination() != dest {
		event, err := c.move.MoveToLocation(dest)
		if err != nil {
			// Can't actually approach (blocked route, zero speed): don't
			// report "still moving" — that would strand the caller waiting
			// on progress that will never happen.
			c.move.CancelFollow()
			return false
		}
		event.FollowTarget = target.ObjectID()
		event.FollowOffset = attackRange
		c.self.BroadcastMove(event)
		c.addPositionUpdate()
	}
	return true
}

// MoveHome requests movement toward home, broadcasts the move, and
// registers for correction ticks the same way any other movement request
// does — otherwise this controller's world presence would stay at the
// stale pre-move cell for the entire walk back. A blocked or unreachable
// route is silently dropped — matching a return-home attempt that simply
// can't make progress this tick, not an application error.
func (c *Controller) MoveHome(home location.Location) {
	event, err := c.move.MoveToLocation(home)
	if err != nil {
		return
	}
	c.self.BroadcastMove(event)
	c.addPositionUpdate()
}

// Stop cancels any active follow task and any movement already under way,
// broadcasting a stop-in-place packet when there was movement to cancel —
// otherwise a client that already received the move request keeps walking
// toward the stale destination until it separately resyncs.
func (c *Controller) Stop() {
	wasMoving := c.move.Moving() || c.move.Following()
	c.move.CancelFollow()
	c.move.CancelMove()
	c.removePositionUpdate()
	if wasMoving {
		c.self.BroadcastStop()
	}
}

// SetArrived records the callback invoked once movement this controller
// started reaches its destination. A nil callback (the default) makes
// arrival a no-op.
func (c *Controller) SetArrived(arrived func()) {
	c.move.SetArrivedHook(func() {
		c.removePositionUpdate()
		if arrived != nil {
			arrived()
		}
	})
}

// PositionUpdate advances one movement correction tick, syncing this
// controller's world presence to the newly interpolated position. It does
// not rebroadcast a movement packet: clients interpolate from the single
// move-start packet already sent by whichever call began this move, and
// resending it every tick would restart that client-side animation instead
// of just correcting server-side state. It returns false once the move has
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
	if !moving {
		if !c.move.Moving() {
			c.removePositionUpdate()
		}
		return c.move.Moving()
	}
	c.self.SyncPosition(event.Origin)
	return true
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
