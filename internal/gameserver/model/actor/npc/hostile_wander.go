package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// minWanderOffset is Npc.moveFromSpawnPointUsingRandomOffset's "offset
// isn't noticeable" cutoff: offsets below this do not start a walk.
const minWanderOffset = 10

// IdleWander reports the idle-wander timer and desire weight for this
// NPC's template. ok is false when the template does not opt into wander
// from an empty desire list.
func (h *Hostile) IdleWander() (timer int, weight float64, ok bool) {
	if h == nil || h.Instance == nil || h.Instance.Template == nil {
		return 0, 0, false
	}
	return IdleWanderParams(h.Instance.Template.ID)
}

// ShouldIdleWander reports whether an empty desire queue should become a
// wander desire. Eligibility is per template id, not instance kind.
func (h *Hostile) ShouldIdleWander() bool {
	_, _, ok := h.IdleWander()
	return ok
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
