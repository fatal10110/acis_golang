package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

var _ clanhall.Treasury = (*GameClientLink)(nil)

// wireClanHallZones has every clan hall's grounds show its decorations to
// a player entering them.
func (l *GameClientLink) wireClanHallZones() {
	if l.zones == nil {
		return
	}
	for _, hall := range zone.OfKind[*zone.ClanHall](l.zones) {
		hallID := int32(hall.ResidenceID)
		hall.ShowInterior = func(a zone.Actor) { l.showClanHallInterior(hallID, a) }
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
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Msg("clan hall: restore clan warehouse for a function fee")
		return false
	}
	end := l.itemInstances.BeginOperation(clanID)
	defer end()
	if adena > wh.Adena() {
		return false
	}
	paid := wh.DestroyByTemplateID(item.AdenaID, adena)
	if paid == nil {
		return false
	}
	l.applyPersistActions([]invops.Persist{invops.DestroyedOrUpdated(clanID, paid)})
	return true
}
