package hero

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// TableNames names the raid bosses from the npc templates and the castles
// from the castles. A nil table names nothing.
type TableNames struct {
	NPCs    *npc.Table
	Castles *castle.Manager
}

// NpcName implements Names.
func (n TableNames) NpcName(npcID int) (string, bool) {
	if n.NPCs == nil {
		return "", false
	}
	tpl, ok := n.NPCs.Get(npcID)
	if !ok {
		return "", false
	}
	return tpl.Name, true
}

// CastleName implements Names.
func (n TableNames) CastleName(castleID int) (string, bool) {
	if n.Castles == nil {
		return "", false
	}
	c, ok := n.Castles.Get(castleID)
	if !ok {
		return "", false
	}
	return c.Name, true
}
