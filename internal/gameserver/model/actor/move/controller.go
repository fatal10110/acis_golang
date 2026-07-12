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
	move *CreatureMove
	self Located
}

// NewController adapts move for self, the position/footprint of the actor
// move drives.
func NewController(move *CreatureMove, self Located) (*Controller, error) {
	if move == nil {
		return nil, errors.New("move: nil creature move")
	}
	if self == nil {
		return nil, errors.New("move: nil self")
	}
	return &Controller{move: move, self: self}, nil
}

// MaybeStartOffensiveFollow starts or refreshes a follow task toward target
// when it sits farther than attackRange plus both actors' footprints, and
// reports whether the caller should wait for the follow to close distance
// instead of attacking now. A target with no known position/footprint
// can't be followed and reports false.
//
// This does not reproduce the reference behavior's line-of-sight branch (an
// out-of-range NPC that also can't see its target still counts it as
// followable) — no geodata query is wired into a live actor yet.
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
	return true
}

// MoveHome requests movement toward home. A blocked or unreachable route is
// silently dropped — matching a return-home attempt that simply can't make
// progress this tick, not an application error.
func (c *Controller) MoveHome(home location.Location) {
	_, _ = c.move.MoveToLocation(home)
}

// Stop cancels any active follow task.
func (c *Controller) Stop() {
	c.move.CancelFollow()
}
