package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type reviveCaster interface {
	WITBonus() float64
}

type reviveTarget interface {
	Revive(percent float64) bool
}

// expRestorer is implemented by player targets that track exp lost at
// death (player.Character.RestoreExp); other revivable targets don't.
type expRestorer interface {
	RestoreExp(restorePercent float64)
}

// Compile-time proof that a real player satisfies both interfaces above —
// resurrect.go's type assertions once silently missed every live player
// because reviveTarget's Revive dropped Character.Revive's bool return.
var (
	_ reviveTarget = (*player.Character)(nil)
	_ expRestorer  = (*player.Character)(nil)
)

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
		target, ok := obj.(reviveTarget)
		if !ok {
			continue
		}
		// Player.doRevive(double) restores exp before the HP/MP/CP revive
		// (Player.java:6008-6012); RestoreExp self-guards on there being an
		// actual death to restore from.
		if er, ok := obj.(expRestorer); ok {
			er.RestoreExp(percent)
		}
		target.Revive(percent)
	}
}
