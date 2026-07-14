package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

// PassiveFuncs builds the stat functions a passive skill contributes for as
// long as its owner has it learned. Unlike a buff, a passive skill never
// becomes a running Effect and never enters a List: its level's raw Funcs
// are its entire behavior, meant to be attached directly to the learner's
// stat calculator once (on learn, on restoring a saved character, or on
// equipping an item that grants it) and left there until the skill is
// unlearned or the granting item unequipped.
//
// The returned Funcs share one owner identity — def's skill and level — so
// a caller can later remove every one of them together the same way a
// buff's Funcs are removed by their owning Effect.
func PassiveFuncs(def modelskill.Definition) ([]basefunc.Func, error) {
	if def.Activation != modelskill.ActivationPassive {
		return nil, fmt.Errorf("effect: skill %d level %d is not a passive skill", def.ID, def.Level)
	}
	owner := modelskill.Ref{ID: def.ID, Level: def.Level}
	return statFuncs(owner, def.Funcs)
}
