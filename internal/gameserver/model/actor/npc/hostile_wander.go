package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// minWanderOffset is Npc.moveFromSpawnPointUsingRandomOffset's "offset
// isn't noticeable" cutoff: offsets below this do not start a walk.
const minWanderOffset = 10

// ShouldIdleWander reports whether an empty desire queue should become a
// wander desire: MovingAttack defaults on, and hold-position kinds (guards
// and chests) stay put without the per-NPC script overlay.
func (h *Hostile) ShouldIdleWander() bool {
	if h.aiInt("MovingAttack", 1) != 1 {
		return false
	}
	switch hostileKind(h.Instance) {
	case "Guard", "SiegeGuard", "Chest", "HalishaChest":
		return false
	default:
		return true
	}
}

// RealMoveSpeed is the stance-aware move speed used as the wander offset
// basis (walk after thinkWander switches stance).
func (h *Hostile) RealMoveSpeed() float64 {
	return float64(h.moveSpeed())
}

// MoveFromSpawnUsingRandomOffset walks toward a geo-validated point offset
// from the spawn home. No spawn point or a sub-noticeable offset is a no-op.
func (h *Hostile) MoveFromSpawnUsingRandomOffset(offset int) {
	if h.Instance == nil || !h.Instance.HasHome || offset < minWanderOffset {
		return
	}
	from := h.location()
	dest := h.Instance.Home
	dest.X += rnd.GetRange(-offset, offset)
	dest.Y += rnd.GetRange(-offset, offset)
	dest = h.ValidLocation(from.X, from.Y, from.Z, dest.X, dest.Y, dest.Z)
	if dest == from {
		return
	}
	mover, ok := h.move.(interface {
		MoveToLocation(location.Location) (bool, error)
	})
	if !ok {
		return
	}
	_, _ = mover.MoveToLocation(dest)
}
