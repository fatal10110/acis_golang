package network

import (
	"context"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

var _ clanhall.Treasury = (*GameClientLink)(nil)

// wireClanHallZones has every clan hall's grounds show its decorations to
// a player entering them, and throw a player out to a banish point of the
// hall.
func (l *GameClientLink) wireClanHallZones() {
	if l.zones == nil {
		return
	}
	for _, hall := range zone.OfKind[*zone.ClanHall](l.zones) {
		hallID := int32(hall.ResidenceID)
		hall.ShowInterior = func(a zone.Actor) { l.showClanHallInterior(hallID, a) }
		hall.Banish = func(a zone.Actor) { l.banishFromClanHall(hallID, a) }
	}
}

// showClanHallInterior sends the decorations of hall hallID to a, a player
// entering its grounds, ahead of the compass update the same zone check
// sends. A hall the hall data does not know shows nothing.
func (l *GameClientLink) showClanHallInterior(hallID int32, a zone.Actor) {
	za, ok := a.(*liveZoneActor)
	if !ok {
		return
	}
	deco, ok := l.hallFunctions.Decoration(hallID)
	if !ok {
		return
	}
	za.live.SendFrame(serverpackets.FrameClanHallDecoration(serverpackets.ClanHallDecoration{
		HallID:       deco.HallID,
		RestoreHP:    deco.RestoreHP,
		RestoreMP:    deco.RestoreMP,
		RestoreExp:   deco.RestoreExp,
		Teleport:     deco.Teleport,
		Curtains:     deco.Curtains,
		SupportMagic: deco.SupportMagic,
		Fixtures:     deco.Fixtures,
		CreateItem:   deco.CreateItem,
	}))
}

// PayHallFee destroys adena from the warehouse of the clan clanID,
// restoring it first when no member opened it, and reports false, with
// nothing taken, when it holds less or cannot be read.
func (l *GameClientLink) PayHallFee(clanID int32, adena int) bool {
	_, ok := l.TakeAdena(clanID, adena)
	return ok
}

var _ clanhall.Bank = (*GameClientLink)(nil)

// TakeAdena is PayHallFee, also returning the write that lands the
// warehouse's adena row as the fee left it. The fee's own row write is
// queued as every item operation's is; the landing writes the same row
// now, in its place in the row's write order, for a caller that has to
// know it is in the database before it stores what the fee paid for.
func (l *GameClientLink) TakeAdena(clanID int32, adena int) (clanhall.Landing, bool) {
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Msg("clan hall: restore clan warehouse for a function fee")
		return nil, false
	}
	end := l.itemInstances.BeginOperation(clanID)
	defer end()
	if adena > wh.Adena() {
		return nil, false
	}
	paid := wh.DestroyByTemplateID(item.AdenaID, adena)
	if paid == nil {
		return nil, false
	}
	l.applyPersistActions([]invops.Persist{invops.DestroyedOrUpdated(clanID, paid)})
	return l.landItems(paid), true
}

// ReturnAdena adds adena to the warehouse of the clan clanID, restoring it
// first when no member opened it, as much of it as keeps the warehouse's
// adena within a 32-bit count, and returns the write that lands the
// warehouse's adena row now. A warehouse that cannot be read, or a new
// stack without an object id, gets nothing and is logged.
func (l *GameClientLink) ReturnAdena(clanID int32, adena int) clanhall.Landing {
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Int("adena", adena).Msg("clan hall: restore clan warehouse for a refund")
		return nil
	}
	end := l.itemInstances.BeginOperation(clanID)
	defer end()
	adena = min(adena, math.MaxInt32-wh.Adena())
	if adena <= 0 {
		return nil
	}
	var id int32
	if wh.Adena() == 0 {
		if id, err = l.nextObjectID(); err != nil {
			l.log.Error().Err(err).Int32("clan_id", clanID).Int("adena", adena).Msg("clan hall: no object id for a refund")
			return nil
		}
	}
	stack := wh.AddNew(item.AdenaID, adena, id)
	if stack == nil {
		if id != 0 {
			l.releaseObjectID(id)
		}
		return nil
	}
	return l.landItems(stack)
}

// landItems returns the write that lands items' rows now, each in its
// place in its row's write order (task.ItemInstances.UpdateItems), or nil
// without item persistence.
func (l *GameClientLink) landItems(items ...*item.Instance) clanhall.Landing {
	instances := l.itemInstances
	if instances == nil || l.items == nil {
		return nil
	}
	return func(ctx context.Context) error { return instances.UpdateItems(ctx, items) }
}

// clanHallNotices tells the clans' members in the world what happened to
// their halls.
type clanHallNotices struct{ l *GameClientLink }

// ClanHallNotifier returns the notifier telling the clans' members
// through l.
func ClanHallNotifier(l *GameClientLink) clanhall.Notifier { return clanHallNotices{l: l} }

// HallChanged refreshes the clan header of cl's members.
func (n clanHallNotices) HallChanged(cl *clan.Clan) {
	n.l.broadcastToClan(cl, 0, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
}

// TellClan sends notice's system message to cl's members.
func (n clanHallNotices) TellClan(cl *clan.Clan, notice clanhall.Notice) {
	var build func() wire.Frame
	switch notice.Kind {
	case clanhall.NoticeAwarded:
		build = func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanHallAwardedToClanS1, notice.Clan)
		}
	case clanhall.NoticeNotSold:
		build = func() wire.Frame { return serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanHallNotSold) }
	case clanhall.NoticeFeeDue:
		build = func() wire.Frame {
			return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageClanHallPaymentDueTomorrowS1, int32(notice.Lease))
		}
	case clanhall.NoticeFeeOverdue:
		build = func() wire.Frame {
			return serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanHallFeeOverdueOwnershipLost)
		}
	default:
		return
	}
	n.l.broadcastToClan(cl, 0, build)
}

// hallOccupants are the players inside the grounds of hall hallID: the
// first clan hall zone the zone data gives that hall.
func (l *GameClientLink) hallOccupants(hallID int32) []zone.Actor {
	if l.zones == nil {
		return nil
	}
	var grounds *zone.ClanHall
	for _, hall := range zone.OfKind[*zone.ClanHall](l.zones) {
		if int32(hall.ResidenceID) == hallID {
			grounds = hall
			break
		}
	}
	if grounds == nil {
		return nil
	}
	var players []zone.Actor
	for _, a := range grounds.Occupants() {
		if _, ok := a.(*liveZoneActor); ok {
			players = append(players, a)
		}
	}
	return players
}
