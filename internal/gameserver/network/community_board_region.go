package network

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
)

// boardRegion runs a region board command: _bbsloc shows the castle list,
// _bbsloc;<id> castle id's page. A command naming no castle id, an id that
// does not read or a castle that does not exist shows nothing.
func (l *GameClientLink) boardRegion(live *livePlayer, command string) {
	if command == "_bbsloc" {
		l.showRegionList(live)
		return
	}
	tokens := bbs.Tokens(command, ";")
	if len(tokens) < 2 {
		return
	}
	id, err := commons.ParseInt(tokens[1], 32)
	if err != nil {
		return
	}
	c, ok := l.castles.Get(int(id))
	if !ok {
		return
	}
	l.showRegionCastle(live, c)
}

// showRegionList shows every castle, by id.
func (l *GameClientLink) showRegionList(live *livePlayer) {
	page, ok := l.boardPage(bbs.RegionListPage)
	if !ok {
		return
	}
	castles := l.castles.All()
	slices.SortFunc(castles, func(a, b *castle.Castle) int { return cmp.Compare(a.ID, b.ID) })
	cards := make([]bbs.RegionCastle, len(castles))
	for i, c := range castles {
		cards[i] = l.regionCastle(c)
	}
	l.sendBoard(live, bbs.RenderRegionList(page, cards))
}

// showRegionCastle shows c's page with the clan halls whose town is named
// as the castle is.
func (l *GameClientLink) showRegionCastle(live *livePlayer, c *castle.Castle) {
	page, ok := l.boardPage(bbs.RegionCastlePage)
	if !ok {
		return
	}
	var halls []bbs.RegionHall
	for _, h := range l.halls.InTown(c.Name) {
		halls = append(halls, bbs.RegionHall{Name: h.Name, Owner: l.regionOwner(h.OwnerID)})
	}
	l.sendBoard(live, bbs.RenderRegionCastle(page, l.regionCastle(c), halls))
}

// regionCastle is c as the region board shows it.
func (l *GameClientLink) regionCastle(c *castle.Castle) bbs.RegionCastle {
	return bbs.RegionCastle{
		ID: c.ID, Name: c.Name, TaxPercent: c.CurrentTaxPercent(), SiegeDate: c.SiegeDate(),
		Owner: l.regionOwner(c.OwnerID()),
	}
}

// regionOwner is clan id as the region board shows it, nil when no clan
// has that id.
func (l *GameClientLink) regionOwner(id int32) *bbs.RegionOwner {
	if id == 0 {
		return nil
	}
	cl, ok := l.clanService().Table().Get(id)
	if !ok {
		return nil
	}
	info := cl.Info()
	return &bbs.RegionOwner{ID: info.ID, Name: info.Name, LeaderName: info.LeaderName, AllyID: info.AllyID, AllyName: info.AllyName}
}
