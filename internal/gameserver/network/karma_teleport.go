package network

import (
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func isTeleportOrRecall(skillType string) bool {
	return skillType == "TELEPORT" || skillType == "RECALL"
}

// itemBlockedByKarmaTeleport reports whether tmpl attaches a TELEPORT or
// RECALL skill, mirroring the reference's item.getSkills() scan.
func itemBlockedByKarmaTeleport(tmpl *item.Template, defs actorcast.Definitions) bool {
	if tmpl == nil {
		return false
	}
	for _, ref := range tmpl.AttachedSkills {
		def, ok := defs.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: int(ref.Level)})
		if ok && isTeleportOrRecall(def.SkillType) {
			return true
		}
	}
	return false
}
