package item

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// isTeleportOrRecallSkillType reports whether skillType is the TELEPORT or
// RECALL classification that the item-use gate checks against every skill
// the item attaches.
func isTeleportOrRecallSkillType(skillType string) bool {
	return skillType == "TELEPORT" || skillType == "RECALL"
}

// isRecallSkillType reports whether skillType is the RECALL classification
// that the direct-cast gate checks; unlike the item-use gate, a direct cast
// does not also gate TELEPORT.
func isRecallSkillType(skillType string) bool {
	return skillType == "RECALL"
}

// karmaBlocksTeleport reports whether the KarmaPlayerCanTeleport setting
// gates a positive-karma actor away from teleport/recall use: both the
// item-use and direct-cast gates share the `!KarmaPlayerCanTeleport &&
// karma > 0` guard.
func karmaBlocksTeleport(karma int, karmaPlayerCanTeleport bool) bool {
	return !karmaPlayerCanTeleport && karma > 0
}

// ItemBlockedByKarmaTeleport reports whether a positive-karma actor is
// blocked from using tmpl because it attaches a TELEPORT or RECALL skill.
func ItemBlockedByKarmaTeleport(tmpl *modelitem.Template, defs actorcast.Definitions, karma int, karmaPlayerCanTeleport bool) bool {
	if tmpl == nil || !karmaBlocksTeleport(karma, karmaPlayerCanTeleport) {
		return false
	}
	for _, ref := range tmpl.AttachedSkills {
		def, ok := defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if ok && isTeleportOrRecallSkillType(def.SkillType) {
			return true
		}
	}
	return false
}

// RecallCastBlockedByKarma reports whether a positive-karma actor is blocked
// from directly casting a RECALL skill.
func RecallCastBlockedByKarma(skillType string, karma int, karmaPlayerCanTeleport bool) bool {
	return isRecallSkillType(skillType) && karmaBlocksTeleport(karma, karmaPlayerCanTeleport)
}
