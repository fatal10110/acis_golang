package item

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// IsTeleportOrRecallSkillType reports whether skillType is the reference's
// TELEPORT or RECALL classification, gated behind Config.KARMA_PLAYER_CAN_TELEPORT
// (UseItem.runImpl's item.getSkills() scan in the Java oracle).
func IsTeleportOrRecallSkillType(skillType string) bool {
	return skillType == "TELEPORT" || skillType == "RECALL"
}

// IsRecallSkillType reports whether skillType is the reference's RECALL
// classification, gated behind Config.KARMA_PLAYER_CAN_TELEPORT
// (RequestMagicSkillUse.runImpl's direct-cast check in the Java oracle, which
// unlike UseItem.runImpl does not also gate TELEPORT).
func IsRecallSkillType(skillType string) bool {
	return skillType == "RECALL"
}

// BlockedByKarmaTeleport reports whether tmpl attaches a TELEPORT or RECALL
// skill, mirroring the reference's item.getSkills() scan in UseItem.runImpl.
func BlockedByKarmaTeleport(tmpl *modelitem.Template, defs actorcast.Definitions) bool {
	if tmpl == nil {
		return false
	}
	for _, ref := range tmpl.AttachedSkills {
		def, ok := defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if ok && IsTeleportOrRecallSkillType(def.SkillType) {
			return true
		}
	}
	return false
}
