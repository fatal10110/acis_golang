package network

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// auctionPageLimit is how many halls one page of the auction list shows.
const auctionPageLimit = 15

// The auction pages' date formats: an auction's end, and the list's end
// dates and bid times.
const (
	auctionEndLayout = "02-01-2006 15:04"
	auctionDayLayout = "2006-01-02"
)

// auctioneerBypass runs command, an auctioneer's own dialog command, for
// live at f. With no hall up for auction every command is refused with a
// notice. Anyone may page through the list, open a hall's details, see the
// map and go back to the first page. Every other command needs a clan
// member holding the auction right, and is otherwise answered with the
// list's first page and a notice: bidding, the bidders list, the bidder's
// and the seller's own pages, and cancelling a bid or a sale. Setting up
// and confirming a sale also need the hall's lease in the clan warehouse.
// A malformed number answers nothing.
func (l *GameClientLink) auctioneerBypass(live *livePlayer, f *npc.Folk, command string) {
	tokens := strings.FieldsFunc(command, func(r rune) bool { return r == ' ' })
	if len(tokens) == 0 {
		return
	}
	actual, val := tokens[0], ""
	if len(tokens) > 1 {
		val = tokens[1]
	}
	if len(l.halls.Auctionable()) == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoClanHallsUpForAuction))
		return
	}
	switch strings.ToLower(actual) {
	case "list":
		l.showAuctionList(live, f, val)
		return
	case "bidding":
		l.showAuctionInfo(live, f, val)
		return
	case "location":
		page := l.setPage("data/html/auction/map_agit_" + l.agitMap(live) + ".htm")
		page = strings.ReplaceAll(page, "%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_start")
		sendFilledHTML(live, f.ObjectID(), page, 0)
		return
	case "start":
		sendFilledHTML(live, f.ObjectID(), f.AuctioneerChat(setPages{l.html}), 0)
		return
	}
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || !cl.HasPrivilege(live.ObjectID(), clan.PrivHallAuction) {
		l.showAuctionList(live, f, "1")
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotParticipateInAuction))
		return
	}
	switch strings.ToLower(actual) {
	case "bid":
		l.placeHallBid(live, cl, tokens)
	case "bid1":
		l.showBidForm(live, f, cl, val)
	case "bidlist":
		l.showBidderList(live, f, cl, val)
	case "selecteditems":
		l.showSelectedItems(live, f, cl)
	case "cancelbid":
		l.showBidCancel(live, f, cl)
	case "docancelbid":
		if l.halls.CancelBid(cl) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCanceledBid))
		}
	case "cancelauction":
		l.showSaleCancel(live, f, cl)
	case "docancelauction":
		if l.halls.CancelSale(cl) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCanceledBid))
		}
		sendFilledHTML(live, f.ObjectID(), f.AuctioneerChat(setPages{l.html}), 0)
	case "sale":
		l.showSaleForm(live, f, cl)
	case "rebid":
		l.showRebidForm(live, f, cl)
	default:
		l.saleCommand(live, f, cl, strings.ToLower(actual), tokens)
	}
}

// saleCommand runs the commands that set up a sale: the price and period
// form, the sale's summary and its confirmation. Each first needs the
// hall's lease in the clan warehouse: short of it, the clan's own page is
// shown with a notice. A clan owning no hall is answered nothing.
func (l *GameClientLink) saleCommand(live *livePlayer, f *npc.Folk, cl *clan.Clan, command string, tokens []string) {
	hallID, ok := l.halls.ByOwner(cl.ID())
	if !ok {
		l.log.Debug().Int32("clan_id", cl.ID()).Str("command", command).Msg("auction: sale command from a clan owning no hall")
		return
	}
	v, ok := l.halls.View(hallID)
	if !ok {
		return
	}
	if l.clanAdena(cl.ID()) < v.Lease {
		l.showSelectedItems(live, f, cl)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughAdenaInClanWarehouse))
		return
	}
	switch command {
	case "auction":
		if len(tokens) < 2 {
			return
		}
		days, err := commons.ParseInt(tokens[1], 32)
		if err != nil {
			return
		}
		minBid := int64(0)
		if len(tokens) > 2 {
			if minBid, err = commons.ParseInt(tokens[2], 32); err != nil {
				return
			}
		}
		end, ok := l.halls.DraftSale(cl, int(days), int(minBid))
		if !ok {
			return
		}
		page := fillPage(l.setPage("data/html/auction/AgitSale3.htm"),
			"%x%", tokens[1],
			"%AGIT_AUCTION_END%", auctionTime(end, auctionEndLayout),
			"%AGIT_AUCTION_MINBID%", strconv.Itoa(v.AuctionMin),
			"%AGIT_AUCTION_MIN%", strconv.FormatInt(minBid, 10),
			"%AGIT_AUCTION_DESC%", v.Description,
			"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_sale2",
			"%objectId%", objID(f))
		sendFilledHTML(live, f.ObjectID(), page, 0)
	case "confirmauction":
		switch l.halls.ConfirmSale(cl) {
		case clanhall.SaleRegistered:
			l.showSelectedItems(live, f, cl)
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageRegisteredForClanHall))
		case clanhall.SaleNoAdena:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughAdenaInClanWarehouse))
		case clanhall.SaleIgnored:
		}
	case "sale2":
		page := fillPage(l.setPage("data/html/auction/AgitSale2.htm"),
			"%AGIT_LAST_PRICE%", strconv.Itoa(v.Lease),
			"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_sale",
			"%objectId%", objID(f))
		sendFilledHTML(live, f.ObjectID(), page, 0)
	default:
		l.log.Debug().Int("npc_id", f.NpcID()).Str("command", command).Msg("bypass: auctioneer command not modeled")
	}
}

// placeHallBid bids the amount tokens name on the hall they name, as cl.
// A missing amount bids 0; a hall or amount that does not parse answers
// nothing.
func (l *GameClientLink) placeHallBid(live *livePlayer, cl *clan.Clan, tokens []string) {
	if len(tokens) < 2 {
		return
	}
	hallID, err := commons.ParseInt(tokens[1], 32)
	if err != nil {
		return
	}
	amount := int64(0)
	if len(tokens) > 2 {
		if amount, err = commons.ParseInt(tokens[2], 32); err != nil {
			return
		}
	}
	var message int
	switch l.halls.Bid(int32(hallID), cl, live.Name, int(amount)) {
	case clanhall.BidPlaced:
		message = serverpackets.SystemMessageBidInClanHallAuction
	case clanhall.BidTooLow:
		message = serverpackets.SystemMessageBidPriceMustBeHigher
	case clanhall.BidNoAdena:
		message = serverpackets.SystemMessageNotEnoughAdenaInClanWarehouse
	case clanhall.BidClanTooLow:
		message = serverpackets.SystemMessageAuctionOnlyClanLevel2Higher
	case clanhall.BidNotAllowed:
		message = serverpackets.SystemMessageCannotParticipateInAuction
	case clanhall.BidElsewhere:
		message = serverpackets.SystemMessageAlreadySubmittedBid
	default:
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(message))
}

// showBidForm opens the bid form of hall val for cl: refused, with the
// list's first page and a notice, for a clan below level 2, one owning a
// hall or one that bid on another hall.
func (l *GameClientLink) showBidForm(live *livePlayer, f *npc.Folk, cl *clan.Clan, val string) {
	if val == "" {
		return
	}
	hallID, err := commons.ParseInt(val, 32)
	if err != nil {
		return
	}
	refusal := 0
	switch at := l.halls.BidAt(cl.ID()); {
	case cl.Level() < 2:
		refusal = serverpackets.SystemMessageAuctionOnlyClanLevel2Higher
	case cl.HallID() > 0:
		refusal = serverpackets.SystemMessageCannotParticipateInAuction
	case at > 0 && at != int32(hallID):
		refusal = serverpackets.SystemMessageAlreadySubmittedBid
	}
	if refusal != 0 {
		l.showAuctionList(live, f, "1")
		live.SendFrame(serverpackets.FrameSystemMessage(refusal))
		return
	}
	v, ok := l.halls.View(int32(hallID))
	if !ok || !v.Auction {
		return
	}
	page := fillPage(l.setPage("data/html/auction/AgitBid1.htm"),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_bidding "+val,
		"%PLEDGE_ADENA%", strconv.Itoa(l.clanAdena(cl.ID())),
		"%AGIT_AUCTION_MINBID%", strconv.Itoa(v.MinBid),
		"npc_%objectId%_bid", "npc_"+objID(f)+"_bid "+val)
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showBidderList opens the bidders of hall val, or of the hall cl bid on
// without val. Its Back leads the selling clan to its own page and
// everyone else to the hall's details.
func (l *GameClientLink) showBidderList(live *livePlayer, f *npc.Folk, cl *clan.Clan, val string) {
	hallID := int64(l.halls.BidAt(cl.ID()))
	if val != "" {
		var err error
		if hallID, err = commons.ParseInt(val, 32); err != nil {
			return
		}
	}
	v, ok := l.halls.View(int32(hallID))
	if !ok || !v.Auction {
		return
	}
	seller := v.Seller != nil && strings.EqualFold(v.Seller.ClanName, cl.Name())
	var sb strings.Builder
	for _, b := range v.Bidders {
		sb.WriteString("<tr><td width=90 align=center>" + b.ClanName + "</td><td width=90 align=center>" + b.Name +
			"</td><td width=90 align=center>" + auctionTime(b.Time, auctionDayLayout) + "</td></tr>")
	}
	back := "bypass -h npc_" + objID(f) + "_bidding " + strconv.FormatInt(hallID, 10)
	if seller {
		back = "bypass -h npc_" + objID(f) + "_selectedItems"
	}
	page := fillPage(l.setPage("data/html/auction/AgitBidderList.htm"),
		"%AGIT_LIST%", sb.String(),
		"%AGIT_LINK_BACK%", back,
		"%objectId%", objID(f))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showBidCancel opens the confirmation of cancelling cl's bid: the bid and
// what comes back of it. A clan without a bid in the auction is answered
// nothing.
func (l *GameClientLink) showBidCancel(live *livePlayer, f *npc.Folk, cl *clan.Clan) {
	v, ok := l.halls.View(l.halls.BidAt(cl.ID()))
	if !ok || !v.Auction {
		return
	}
	b, ok := v.Bid(cl.ID())
	if !ok {
		return
	}
	page := fillPage(l.setPage("data/html/auction/AgitBidCancel.htm"),
		"%AGIT_BID%", strconv.Itoa(b.Bid),
		"%AGIT_BID_REMAIN%", strconv.Itoa(clanhall.TaxedRefund(b.Bid)),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_selectedItems",
		"%objectId%", objID(f))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showSaleCancel opens the confirmation of cancelling the sale of the hall
// cl owns, naming the deposit lost. A clan owning no hall is answered
// nothing.
func (l *GameClientLink) showSaleCancel(live *livePlayer, f *npc.Folk, cl *clan.Clan) {
	v, ok := l.ownedHallView(cl)
	if !ok {
		return
	}
	page := fillPage(l.setPage("data/html/auction/AgitSaleCancel.htm"),
		"%AGIT_DEPOSIT%", strconv.Itoa(v.Lease),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_selectedItems",
		"%objectId%", objID(f))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showSaleForm opens the first sale page of the hall cl owns: the deposit
// and the clan warehouse's adena. A clan owning no hall is answered
// nothing.
func (l *GameClientLink) showSaleForm(live *livePlayer, f *npc.Folk, cl *clan.Clan) {
	v, ok := l.ownedHallView(cl)
	if !ok {
		return
	}
	page := fillPage(l.setPage("data/html/auction/AgitSale1.htm"),
		"%AGIT_DEPOSIT%", strconv.Itoa(v.Lease),
		"%AGIT_PLEDGE_ADENA%", strconv.Itoa(l.clanAdena(cl.ID())),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_selectedItems",
		"%objectId%", objID(f))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showRebidForm opens the rebid page of the hall cl bid on: its bid, the
// hall's minimum and the auction's end. A clan without a bid in the
// auction is answered nothing.
func (l *GameClientLink) showRebidForm(live *livePlayer, f *npc.Folk, cl *clan.Clan) {
	v, ok := l.halls.View(l.halls.BidAt(cl.ID()))
	if !ok || !v.Auction {
		return
	}
	b, ok := v.Bid(cl.ID())
	if !ok {
		return
	}
	page := fillPage(l.setPage("data/html/auction/AgitBid2.htm"),
		"%AGIT_AUCTION_BID%", strconv.Itoa(b.Bid),
		"%AGIT_AUCTION_MINBID%", strconv.Itoa(v.AuctionMin),
		"%AGIT_AUCTION_END%", auctionTime(v.EndDate, auctionEndLayout),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_selectedItems",
		"npc_%objectId%_bid1", "npc_"+objID(f)+"_bid1 "+strconv.Itoa(int(v.ID)))
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// ownedHallView returns the hall cl owns.
func (l *GameClientLink) ownedHallView(cl *clan.Clan) (clanhall.HallView, bool) {
	hallID, ok := l.halls.ByOwner(cl.ID())
	if !ok {
		return clanhall.HallView{}, false
	}
	return l.halls.View(hallID)
}

// showAuctionList opens page val (the first when empty) of the halls up
// for auction, 15 a page, with a link to every page. A page that does not
// parse or holds no hall answers nothing.
func (l *GameClientLink) showAuctionList(live *livePlayer, f *npc.Folk, val string) {
	halls := l.halls.Auctionable()
	page := int64(1)
	if val != "" {
		var err error
		if page, err = commons.ParseInt(val, 32); err != nil {
			return
		}
	}
	from, to := (page-1)*auctionPageLimit, min(page*auctionPageLimit, int64(len(halls)))
	if from < 0 || from > to {
		return
	}
	var sb strings.Builder
	sb.WriteString("<table width=280>")
	for _, v := range halls[from:to] {
		sb.WriteString("<tr><td><font color=\"aaaaff\">" + v.Town + "</font></td><td><font color=\"ffffaa\"><a action=\"bypass -h npc_" +
			objID(f) + "_bidding " + strconv.Itoa(int(v.ID)) + "\">" + v.Name + " [" + strconv.Itoa(len(v.Bidders)) + "]</a></font></td><td>" +
			auctionTime(v.EndDate, auctionDayLayout) + "</td><td><font color=\"aaffff\">" + strconv.Itoa(v.MinBid) + "</font></td></tr>")
	}
	sb.WriteString("</table><table width=280><tr>")
	pages := len(halls) / auctionPageLimit
	if len(halls)%auctionPageLimit != 0 {
		pages++
	}
	for j := 1; j <= pages; j++ {
		sb.WriteString("<td align=center><a action=\"bypass -h npc_" + objID(f) + "_list " + strconv.Itoa(j) + "\"> Page " + strconv.Itoa(j) + " </a></td>")
	}
	sb.WriteString("</tr></table>")
	html := fillPage(l.setPage("data/html/auction/AgitAuctionList.htm"),
		"%AGIT_LIST%", sb.String(),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_start")
	sendFilledHTML(live, f.ObjectID(), html, 0)
}

// showAuctionInfo opens the details of hall val's auction. A hall that
// does not parse, is unknown or holds no auction answers nothing.
func (l *GameClientLink) showAuctionInfo(live *livePlayer, f *npc.Folk, val string) {
	if val == "" {
		return
	}
	hallID, err := commons.ParseInt(val, 32)
	if err != nil {
		return
	}
	v, ok := l.halls.View(int32(hallID))
	if !ok || !v.Auction {
		return
	}
	ownerName, ownerMaster := "", ""
	if v.Seller != nil {
		ownerName, ownerMaster = v.Seller.ClanName, v.Seller.Name
	}
	id := strconv.Itoa(int(v.ID))
	page := fillPage(l.setPage("data/html/auction/AgitAuctionInfo.htm"),
		"%AGIT_NAME%", v.Name,
		"%AGIT_SIZE%", strconv.Itoa(v.Size),
		"%AGIT_LEASE%", strconv.Itoa(v.Lease),
		"%AGIT_LOCATION%", v.Town,
		"%AGIT_AUCTION_END%", auctionTime(v.EndDate, auctionEndLayout),
		"%AGIT_AUCTION_REMAIN%", auctionRemaining(v.EndDate-l.auctionNow()),
		"%AGIT_AUCTION_MINBID%", strconv.Itoa(v.MinBid),
		"%AGIT_AUCTION_COUNT%", strconv.Itoa(len(v.Bidders)),
		"%AGIT_AUCTION_DESC%", v.Description,
		"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_list",
		"%AGIT_LINK_BIDLIST%", "bypass -h npc_"+objID(f)+"_bidlist "+id,
		"%AGIT_LINK_RE%", "bypass -h npc_"+objID(f)+"_bid1 "+id,
		"%OWNER_PLEDGE_NAME%", ownerName,
		"%OWNER_PLEDGE_MASTER%", ownerMaster)
	sendFilledHTML(live, f.ObjectID(), page, 0)
}

// showSelectedItems opens cl's own page: for a clan owning no hall and
// bidding, its bid; for a clan owning a hall, the hall and, when it is for
// sale, the sale. A clan doing neither sees the list's first page and a
// notice.
func (l *GameClientLink) showSelectedItems(live *livePlayer, f *npc.Folk, cl *clan.Clan) {
	if at := l.halls.BidAt(cl.ID()); cl.HallID() == 0 && at > 0 {
		v, ok := l.halls.View(at)
		if !ok || !v.Auction {
			return
		}
		b, ok := v.Bid(cl.ID())
		if !ok {
			return
		}
		ownerName, ownerMaster, minBid := "", "", v.AuctionMin
		if v.Seller != nil {
			ownerName, ownerMaster, minBid = v.Seller.ClanName, v.Seller.Name, v.Seller.Bid
		}
		page := fillPage(l.setPage("data/html/auction/AgitBidInfo.htm"),
			"%AGIT_NAME%", v.Name,
			"%AGIT_SIZE%", strconv.Itoa(v.Size),
			"%AGIT_LEASE%", strconv.Itoa(v.Lease),
			"%AGIT_LOCATION%", v.Town,
			"%AGIT_AUCTION_END%", auctionTime(v.EndDate, auctionEndLayout),
			"%AGIT_AUCTION_REMAIN%", auctionRemaining(v.EndDate-l.auctionNow()),
			"%AGIT_AUCTION_MYBID%", strconv.Itoa(b.Bid),
			"%AGIT_AUCTION_DESC%", v.Description,
			"%objectId%", objID(f),
			"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_start",
			"%OWNER_PLEDGE_NAME%", ownerName,
			"%OWNER_PLEDGE_MASTER%", ownerMaster,
			"%AGIT_AUCTION_MINBID%", strconv.Itoa(minBid))
		sendFilledHTML(live, f.ObjectID(), page, 0)
		return
	}
	if hallID := cl.HallID(); hallID > 0 {
		v, ok := l.halls.View(hallID)
		if !ok || !v.Auction {
			return
		}
		var page string
		if v.Seller != nil {
			page = fillPage(l.setPage("data/html/auction/AgitSaleInfo.htm"),
				"%AGIT_NAME%", v.Name,
				"%AGIT_OWNER_PLEDGE_NAME%", v.Seller.ClanName,
				"%OWNER_PLEDGE_MASTER%", v.Seller.Name,
				"%AGIT_SIZE%", strconv.Itoa(v.Size),
				"%AGIT_LEASE%", strconv.Itoa(v.Lease),
				"%AGIT_LOCATION%", v.Town,
				"%AGIT_AUCTION_END%", auctionTime(v.EndDate, auctionEndLayout),
				"%AGIT_AUCTION_REMAIN%", auctionRemaining(v.EndDate-l.auctionNow()),
				"%AGIT_AUCTION_MINBID%", strconv.Itoa(v.Seller.Bid),
				"%AGIT_AUCTION_BIDCOUNT%", strconv.Itoa(len(v.Bidders)),
				"%AGIT_AUCTION_DESC%", v.Description,
				"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_start",
				"%id%", strconv.Itoa(int(v.ID)),
				"%objectId%", objID(f))
		} else {
			info := cl.Info()
			page = fillPage(l.setPage("data/html/auction/AgitInfo.htm"),
				"%AGIT_NAME%", v.Name,
				"%AGIT_OWNER_PLEDGE_NAME%", info.Name,
				"%OWNER_PLEDGE_MASTER%", info.LeaderName,
				"%AGIT_SIZE%", strconv.Itoa(v.Size),
				"%AGIT_LEASE%", strconv.Itoa(v.Lease),
				"%AGIT_LOCATION%", v.Town,
				"%AGIT_LINK_BACK%", "bypass -h npc_"+objID(f)+"_start",
				"%objectId%", objID(f))
		}
		sendFilledHTML(live, f.ObjectID(), page, 0)
		return
	}
	l.showAuctionList(live, f, "1")
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoOfferingsOwnOrMadeBidFor))
}

// clanAdena is the adena in the warehouse of the clan clanID, 0 when it
// cannot be read.
func (l *GameClientLink) clanAdena(clanID int32) int {
	wh, err := l.clanWarehouse(clanID)
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", clanID).Msg("auction: restore clan warehouse")
		return 0
	}
	return wh.Adena()
}

// agitMap names the town map the location page shows: the one of the
// restart point whose map regions cover live's tile, Aden's elsewhere.
// It is the plain region point: no restart area and no banned-race
// redirect applies.
func (l *GameClientLink) agitMap(live *livePlayer) string {
	if l.restarts == nil {
		return "aden"
	}
	point, ok := l.restarts.PointAt(live.CurrentLocation())
	if !ok {
		return "aden"
	}
	switch point.LocName {
	case 912:
		return "gludio"
	case 911:
		return "gludin"
	case 916:
		return "dion"
	case 918:
		return "giran"
	case 1537:
		return "rune"
	case 1538:
		return "godard"
	case 1714:
		return "schuttgart"
	}
	return "aden"
}

// auctionNow is the clock the auctions run on.
func (l *GameClientLink) auctionNow() int64 { return l.halls.Now() }

// auctionTime writes ms, Unix milliseconds, in the server's time zone.
func auctionTime(ms int64, layout string) string {
	return time.UnixMilli(ms).Format(layout)
}

// auctionRemaining writes ms as "<h> hours <m> minutes", each truncated
// toward zero.
func auctionRemaining(ms int64) string {
	return strconv.FormatInt(ms/3600000, 10) + " hours " + strconv.FormatInt((ms/60000)%60, 10) + " minutes"
}

// fillPage replaces each pattern of pairs in page, in order, with the
// value after it.
func fillPage(page string, pairs ...string) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		page = strings.ReplaceAll(page, pairs[i], pairs[i+1])
	}
	return page
}

func objID(f *npc.Folk) string { return strconv.Itoa(int(f.ObjectID())) }
