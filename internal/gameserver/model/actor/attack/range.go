package attack

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	standingAttackGrace = 10
	movingAttackGrace   = 60
)

type positionedRadius interface {
	Position() (int, int, int)
	CollisionRadius() float64
}

type movingCombatant interface {
	IsMoving() bool
}

// PhysicalReach is the truncated 2D attack radius: weapon range plus both
// collision radii plus a grace margin (60 if the target is moving, else 10).
// The sum is truncated once after adding the floating-point radii.
func PhysicalReach(attackRange int, attackerRadius, targetRadius float64, targetMoving bool) int {
	grace := standingAttackGrace
	if targetMoving {
		grace = movingAttackGrace
	}
	return int(float64(attackRange) + attackerRadius + targetRadius + float64(grace))
}

// InPhysicalRange reports whether target sits strictly inside the attacker's
// 2D physical reach. Altitude is ignored. A target without a known position
// or collision footprint is out of range.
func InPhysicalRange(from location.Location, attackRange int, attackerRadius float64, target attackable.Combatant) bool {
	other, ok := target.(positionedRadius)
	if !ok {
		return false
	}
	tx, ty, _ := other.Position()
	moving := false
	if m, ok := target.(movingCombatant); ok {
		moving = m.IsMoving()
	}
	reach := PhysicalReach(attackRange, attackerRadius, other.CollisionRadius(), moving)
	return location.In2DRadius(from.X, from.Y, tx, ty, reach)
}
