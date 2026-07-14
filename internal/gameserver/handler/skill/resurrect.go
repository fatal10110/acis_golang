package skill

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"

type reviveCaster interface {
	WITBonus() float64
}

type reviveTarget interface {
	Revive(percent float64)
}

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
		target.Revive(percent)
	}
}
