package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// NPCKindOf returns the kind each NPC template in templates spawns as, for
// Config.KindOf: false for an id with no template.
func NPCKindOf(templates *npc.Table) func(int32) (NPCKind, bool) {
	return func(id int32) (NPCKind, bool) {
		tmpl, ok := templates.Get(int(id))
		if !ok {
			return KindOther, false
		}
		probe := &npc.Instance{Template: tmpl}
		switch {
		case npc.FolkKind(probe):
			return KindFolk, true
		case npc.Attackable(probe):
			return KindHostile, true
		default:
			return KindOther, true
		}
	}
}
