package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// castleManorEffects pays the castle owners' clans and tells their leaders
// what the manor cycle does.
type castleManorEffects struct{ l *GameClientLink }

// CastleManorEffects returns the manor cycle's effects through l.
func CastleManorEffects(l *GameClientLink) castlemanor.Effects { return castleManorEffects{l: l} }

// AddToClanWarehouse adds count of itemID to the warehouse of the clan
// clanID, restoring it first when no member opened it: onto its stack of
// itemID when it holds a stackable one, as a new item otherwise. A
// warehouse that cannot be read, or an item without an object id, gets
// nothing and is logged.
func (e castleManorEffects) AddToClanWarehouse(clanID, itemID int32, count int) {
	l := e.l
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Int32("item_id", itemID).Int("count", count).Msg("manor: restore clan warehouse for crops")
		return
	}
	end := l.itemInstances.BeginOperation(clanID)
	defer end()
	id, err := l.nextObjectID()
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Int32("item_id", itemID).Int("count", count).Msg("manor: no object id for crops")
		return
	}
	if added := wh.AddNew(itemID, count, id); added == nil || added.ObjectID != id {
		l.releaseObjectID(id)
	}
}

// TellManorUpdated tells the player leaderID, when in the world, that the
// manor information was updated.
func (e castleManorEffects) TellManorUpdated(leaderID int32) {
	if live, ok := e.l.livePlayerByID(leaderID); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageManorInformationUpdated))
	}
}
