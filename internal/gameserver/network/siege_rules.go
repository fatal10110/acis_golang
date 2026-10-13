package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// castleBesieged reports whether castle castleID's siege is being fought.
func (l *GameClientLink) castleBesieged(castleID int) bool {
	s, ok := l.sieges.Get(castleID)
	return ok && s.InProgress()
}

// ownCastleBesieged reports whether clan clanID owns a castle whose siege
// is being fought.
func (l *GameClientLink) ownCastleBesieged(clanID int32) bool {
	if clanID == 0 {
		return false
	}
	c, ok := l.castles.ByOwner(clanID)
	return ok && l.castleBesieged(c.ID)
}

// A castle's siege is the one its control towers stand for.
var _ npc.TowerSiege = (*siege.Siege)(nil)

// GuardedSiege returns the siege of the castle inst belongs to (npcCastle),
// the one a siege guard defends or a control tower stands for; nil for
// none.
func (l *GameClientLink) GuardedSiege(inst *npc.Instance) npc.GuardedSiege {
	c, ok := l.npcCastle(inst)
	if !ok {
		return nil
	}
	s, ok := l.sieges.Get(c.ID)
	if !ok {
		return nil
	}
	return s
}
