package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type reviveCaster interface {
	WITBonus() float64
}

// Compile-time proof that a real player is revivable — this handler's type
// assertion once silently missed every live player because its interface
// dropped Character.Revive's bool return.
var _ Player = (*player.Character)(nil)

type resurrectHandler struct{}

func (resurrectHandler) Types() []string { return []string{"RESURRECT"} }

// Use revives every resolved target by the caster's revive-power roll. The
// live game additionally routes a player target through a confirmation
// dialog, and forwards a foreign pet's request to its owner instead of
// reviving it outright — both need the request/response dialog flow, which
// isn't wired yet, so a revivable target here is revived immediately.
func (resurrectHandler) Use(cast Cast) {
	caster, ok := cast.Caster.(reviveCaster)
	if !ok {
		return
	}

	percent := formulas.RevivePower(caster.WITBonus(), float64(cast.Skill.Power))
	for _, obj := range cast.Targets {
		target, ok := asPlayer(obj)
		if !ok {
			continue
		}
		// Player.doRevive(double) restores exp before the HP/MP/CP revive
		// (Player.java:6008-6012); RestoreExp self-guards on there being an
		// actual death to restore from.
		target.RestoreExp(percent)
		target.Revive(percent)
	}
}
