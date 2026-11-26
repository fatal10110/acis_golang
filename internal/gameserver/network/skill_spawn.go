package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// placeSkillNPC places the NPC of template npcID a SPAWN skill calls at
// (x, y) on the ground below z, facing heading, as a spawn that never
// respawns, deleted once despawn has passed when it is positive. A
// decoration is placed on its own; every other NPC joins the live
// population as a standalone spawn. An unknown template or an NPC that
// cannot be placed places nothing and is logged.
func (l *GameClientLink) placeSkillNPC(npcID int32, x, y, z, heading int, despawn time.Duration) {
	var tmpl *npc.Template
	if l.npcs != nil {
		tmpl, _ = l.npcs.Get(int(npcID))
	}
	if tmpl == nil {
		l.log.Error().Int32("npc_id", npcID).Msg("skill spawn: no such npc template")
		return
	}
	if tmpl.Kind() == npc.KindChristmasTree {
		var decoration *npc.Decoration
		if l.ids != nil {
			decoration = l.placeDecoration(tmpl, tmpl.Title, x, y, z, heading, 0)
		}
		if decoration == nil {
			l.log.Error().Int32("npc_id", npcID).Msg("skill spawn: decoration placed nothing")
			return
		}
		if despawn > 0 {
			decoration.Queue().After(despawn, decoration.Despawn)
		}
		return
	}
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		l.log.Error().Int32("npc_id", npcID).Msg("skill spawn: no live npc population")
		return
	}
	if _, err := npcs.SpawnTemporary(tmpl, x, y, z, heading, despawn); err != nil {
		l.log.Error().Err(err).Int32("npc_id", npcID).Msg("skill spawn placed nothing")
	}
}
