package network

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// addNPCLevel starts the title in s, v's NpcInfo, with v's level, when
// npcs.properties ShowNpcLevel is set and v is a monster: "Lv <level>",
// then "* " for a monster with an aggro range or " " for one without, then
// the title s carries. Any other NPC keeps its title.
func (l *GameClientLink) addNPCLevel(v world.Tracked, s *npcinfo.Snapshot) {
	if l == nil || !l.showNPCLevel {
		return
	}
	h, ok := v.(*npc.Hostile)
	if !ok || !h.MonsterKind() {
		return
	}
	mark := " "
	if h.Instance.Template.AggroRange > 0 {
		mark = "* "
	}
	s.Title = "Lv " + strconv.Itoa(h.Level()) + mark + s.Title
}
