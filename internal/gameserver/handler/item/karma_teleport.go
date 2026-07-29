package item

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// isTeleportOrRecallSkillType reports whether skillType is the reference's
// TELEPORT or RECALL classification, scanned by UseItem.runImpl's
// item.getSkills() loop in the Java oracle.
func isTeleportOrRecallSkillType(skillType string) bool {
	return skillType == "TELEPORT" || skillType == "RECALL"
}

// isRecallSkillType reports whether skillType is the reference's RECALL
// classification, checked directly by RequestMagicSkillUse.runImpl in the
// Java oracle, which unlike UseItem.runImpl does not also gate TELEPORT.
func isRecallSkillType(skillType string) bool {
	return skillType == "RECALL"
}

// karmaBlocksTeleport reports whether Config.KARMA_PLAYER_CAN_TELEPORT gates
// a positive-karma actor away from teleport/recall use, per both call sites'
// shared `!KarmaPlayerCanTeleport && karma > 0` guard in the Java oracle.
func karmaBlocksTeleport(karma int, karmaPlayerCanTeleport bool) bool {
	return !karmaPlayerCanTeleport && karma > 0
}

// ItemBlockedByKarmaTeleport reports whether a positive-karma actor is
// blocked from using tmpl because it attaches a TELEPORT or RECALL skill,
// mirroring UseItem.runImpl in the Java oracle.
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
// from directly casting a RECALL skill, mirroring RequestMagicSkillUse.runImpl
// in the Java oracle.
func RecallCastBlockedByKarma(skillType string, karma int, karmaPlayerCanTeleport bool) bool {
	return isRecallSkillType(skillType) && karmaBlocksTeleport(karma, karmaPlayerCanTeleport)
}
