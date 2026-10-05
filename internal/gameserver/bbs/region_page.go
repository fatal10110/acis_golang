package bbs

import (
	"strconv"
	"strings"
	"time"
)

// The region board's pages, under the board's page folder.
const (
	RegionListPage   = "region/castlelist.htm"
	RegionCastlePage = "region/castle.htm"
)

// siegeDateLayout is how the castle page shows the next siege date.
const siegeDateLayout = "2006-01-02 15:04"

// RegionOwner is the clan owning a castle or clan hall as the region board
// shows it.
type RegionOwner struct {
	ID         int32
	Name       string
	LeaderName string
	AllyID     int32
	AllyName   string
}

// RegionCastle is a castle as the region board shows it; Owner is nil for
// a castle no clan owns.
type RegionCastle struct {
	ID   int
	Name string
	// TaxPercent is the tax rate in force.
	TaxPercent int
	// SiegeDate is the next siege date, in Unix milliseconds.
	SiegeDate int64
	Owner     *RegionOwner
}

// RegionHall is a clan hall as the castle page lists it; Owner is nil for
// a free hall.
type RegionHall struct {
	Name  string
	Owner *RegionOwner
}

// clanLink is the clan board link of owner, "None" without one.
func (o *RegionOwner) clanLink() string {
	if o == nil {
		return "None"
	}
	return `<a action="bypass _bbsclan;home;` + strconv.Itoa(int(o.ID)) + `">` + o.Name + "</a>"
}

// allyName is the owner's alliance name, "None" without an owner or an
// alliance.
func (o *RegionOwner) allyName() string {
	if o == nil || o.AllyID <= 0 {
		return "None"
	}
	return o.AllyName
}

// leaderName is the owner's leader name, "None" without an owner.
func (o *RegionOwner) leaderName() string {
	if o == nil {
		return "None"
	}
	return o.LeaderName
}

// RenderRegionList fills page, the castle list, with one row per castle:
// its page link, its owner's clan link, alliance and tax rate in force (0
// for a castle no clan owns).
func RenderRegionList(page string, castles []RegionCastle) string {
	var b strings.Builder
	for _, c := range castles {
		tax := "0"
		if c.Owner != nil {
			tax = strconv.Itoa(c.TaxPercent)
		}
		b.WriteString(`<table><tr><td width=5></td><td width=160><a action="bypass _bbsloc;`)
		b.WriteString(strconv.Itoa(c.ID))
		b.WriteString(`">`)
		b.WriteString(c.Name)
		b.WriteString(`</a></td><td width=160>`)
		b.WriteString(c.Owner.clanLink())
		b.WriteString(`</td><td width=160>`)
		b.WriteString(c.Owner.allyName())
		b.WriteString(`</td><td width=120>`)
		b.WriteString(tax)
		b.WriteString(`</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`)
	}
	return strings.ReplaceAll(page, "%castleList%", b.String())
}

// RenderRegionCastle fills page, a castle's page, with c and halls, the
// clan halls of its town; the hall table shows only when there is one.
// The siege date shows in the server's time zone.
func RenderRegionCastle(page string, c RegionCastle, halls []RegionHall) string {
	page = strings.ReplaceAll(page, "%castleName%", c.Name)
	page = strings.ReplaceAll(page, "%tax%", strconv.Itoa(c.TaxPercent))
	page = strings.ReplaceAll(page, "%lord%", c.Owner.leaderName())
	page = strings.ReplaceAll(page, "%clanName%", c.Owner.clanLink())
	page = strings.ReplaceAll(page, "%allyName%", c.Owner.allyName())
	page = strings.ReplaceAll(page, "%siegeDate%", time.UnixMilli(c.SiegeDate).Format(siegeDateLayout))

	var b strings.Builder
	if len(halls) > 0 {
		b.WriteString(`<br><br><table width=610 bgcolor=A7A19A><tr><td width=5></td><td width=200>Clan Hall Name</td><td width=200>Owning Clan</td><td width=200>Clan Leader Name</td><td width=5></td></tr></table><br1>`)
		for _, h := range halls {
			b.WriteString(`<table><tr><td width=5></td><td width=200>`)
			b.WriteString(h.Name)
			b.WriteString(`</td><td width=200>`)
			b.WriteString(h.Owner.clanLink())
			b.WriteString(`</td><td width=200>`)
			b.WriteString(h.Owner.leaderName())
			b.WriteString(`</td><td width=5></td></tr></table><br1><img src="L2UI.Squaregray" width=605 height=1><br1>`)
		}
	}
	return strings.ReplaceAll(page, "%hallsList%", b.String())
}
