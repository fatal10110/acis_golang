package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
func PassiveFuncs(def modelskill.Definition) ([]Mod, error) {
	if def.Activation != modelskill.ActivationPassive {
		return nil, fmt.Errorf("effect: skill %d level %d is not a passive skill", def.ID, def.Level)
	}
	return SkillStatFuncs(def)
}

// SkillStatFuncs builds def's top-level func templates as Mods owned by the
// skill level, regardless of operate type. NPC XML type="PASSIVE" is a
// template skill kind, not an operate-type gate: Attackable and Playable
// actors attach those func templates even when the skill itself is not
// ActivationPassive.
func SkillStatFuncs(def modelskill.Definition) ([]Mod, error) {
	owner := ModOwnerSkill(modelskill.Ref{ID: def.ID, Level: def.Level})
	return statFuncs(owner, def.Funcs, nil)
}

// skillLookup resolves a loaded skill definition by id and level.
type skillLookup interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// TemplatePassiveMods resolves passives through lookup and builds each
// skill's func templates. Missing id/level pairs and skills whose funcs
// cannot be built (for example enchant funcs without an item owner) are
// skipped. A nil lookup or empty list yields no mods.
func TemplatePassiveMods(lookup skillLookup, passives []modelskill.Ref) []Mod {
	if lookup == nil || len(passives) == 0 {
		return nil
	}
	var mods []Mod
	for _, ref := range passives {
		def, ok := lookup.Definition(ref)
		if !ok {
			continue
		}
		fns, err := SkillStatFuncs(def)
		if err != nil {
			continue
		}
		mods = append(mods, fns...)
	}
	return mods
}
