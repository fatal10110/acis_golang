package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// BroadcastSkillAnimation shows the players around h the cast animation of
// skillID at level from casterID, standing at casterAt, to targetID,
// standing at targetAt. It shows nothing more: no skill is used, and the
// caster and the target may be any creatures.
func (h *Hostile) BroadcastSkillAnimation(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int) {
	h.emit(skillAnimation(casterID, casterAt, targetID, targetAt, skillID, level, hitTime, reuseDelay))
}

// BroadcastSkillAnimation shows the players around f the cast animation of
// skillID at level from casterID, standing at casterAt, to targetID,
// standing at targetAt. It shows nothing more: no skill is used, and the
// caster and the target may be any creatures.
func (f *Folk) BroadcastSkillAnimation(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int) {
	f.emit(skillAnimation(casterID, casterAt, targetID, targetAt, skillID, level, hitTime, reuseDelay))
}

func skillAnimation(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int) event.MagicSkillUse {
	return event.MagicSkillUse{
		CasterID: casterID, CasterAt: casterAt,
		TargetID: targetID, TargetAt: targetAt,
		SkillID: skillID, Level: level, HitTime: hitTime, ReuseDelay: reuseDelay,
	}
}
