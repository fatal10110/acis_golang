package skill

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type facingTarget interface {
	Heading() int
	X() int
	Y() int
	Z() int
}

type jumpCaster interface {
	AbortAll(force bool)
	SetXYZ(x, y, z int)
	BroadcastPosition()
}

type instantJumpHandler struct{}

func (instantJumpHandler) Types() []string { return []string{"INSTANT_JUMP"} }

// Use repositions the caster 25 units behind the first target, facing the
// same direction the target faces, then broadcasts the new position.
func (instantJumpHandler) Use(cast Cast) {
	if len(cast.Targets) == 0 {
		return
	}
	target, ok := cast.Targets[0].(facingTarget)
	if !ok {
		return
	}
	caster, ok := cast.Caster.(jumpCaster)
	if !ok {
		return
	}

	degrees := location.HeadingDegrees(target.Heading()) + 180
	if degrees > 360 {
		degrees -= 360
	}
	radians := math.Pi * degrees / 180

	x := target.X() + int(25*math.Cos(radians))
	y := target.Y() + int(25*math.Sin(radians))

	caster.AbortAll(false)
	caster.SetXYZ(x, y, target.Z())
	caster.BroadcastPosition()
}

type casterPosition interface {
	Position() (x, y, z int)
}

type teleportTarget interface {
	TeleportTo(x, y, z int)
}

type getPlayerHandler struct{}

func (getPlayerHandler) Types() []string { return []string{"GET_PLAYER"} }

// Use pulls every resolved target to the caster's position. The live game
// resolves each target to its acting player (a pet or summon target pulls
// its owner); that indirection belongs to target resolution, not this
// handler, so a target here is teleported directly.
func (getPlayerHandler) Use(cast Cast) {
	if alikeDead(cast.Caster) {
		return
	}
	caster, ok := cast.Caster.(casterPosition)
	if !ok {
		return
	}
	x, y, z := caster.Position()

	for _, obj := range cast.Targets {
		if alikeDead(obj) {
			continue
		}
		target, ok := obj.(teleportTarget)
		if !ok {
			continue
		}
		target.TeleportTo(x, y, z)
	}
}
