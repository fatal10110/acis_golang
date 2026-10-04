package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// leaverCastle returns the castle cl owns, whose items a member leaving cl
// is checked for, reporting false when cl owns none: a clan without a
// castle checks nothing, so a leaver keeps its rank and clan hall items on.
func (l *GameClientLink) leaverCastle(cl *clan.Clan) (*castle.Castle, bool) {
	id := cl.CastleID()
	if id <= 0 {
		return nil, false
	}
	return l.castles.Get(int(id))
}

// checkLeaverItems has live, leaving cl, take off what it may no longer
// wear when cl owns a castle; it runs on live's queue, before live's clan
// state is cleared. The check therefore still sees live in cl, as the
// reference's Castle.checkItemsForMember does, so the castle's circlet and
// the rank items stay on and only an item already failing its conditions
// comes off.
func (l *GameClientLink) checkLeaverItems(live *livePlayer, cl *clan.Clan) {
	if _, ok := l.leaverCastle(cl); ok {
		l.unequipRestrictedItems(live)
	}
}

// unequipLeaverCirclets moves the equipped circlet of cl's castle and the
// Lord's Crown of memberID, an offline member leaving cl, back to its
// inventory, on its persistence lane behind its removal row. A clan
// without a castle moves nothing.
func (l *GameClientLink) unequipLeaverCirclets(cl *clan.Clan, memberID int32) {
	if c, ok := l.leaverCastle(cl); ok {
		l.castles.UnequipCirclets(c, memberID)
	}
}
