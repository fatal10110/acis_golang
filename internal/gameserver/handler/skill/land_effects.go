package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// LandEffects lands def's effects on effected from effector outside any
// cast, as a skill applied directly: no cast, no shot, no shield. A passive
// skill or one without effects lands nothing; otherwise the landing gates
// and rolls of a cast's own effects apply.
func LandEffects(effector effect.Actor, effected Actor, def modelskill.Definition) {
	if def.Activation == modelskill.ActivationPassive || len(def.Effects) == 0 {
		return
	}
	applyEffects(effector, effected, def, def.Effects)
}
