package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminClanHallUsage answers a //ch whose hall id does not parse, or whose
// hall has no owner restart point to teleport to.
const adminClanHallUsage = "Usage: //ch [set|remove|open|close|teleportto|end|siege chId]."

// adminClanHallTowns are the towns the clan hall list shows, in order; a
// hall in another town is not listed.
var adminClanHallTowns = []string{"Aden", "Dion", "Giran", "Gludin", "Gludio", "Goddard", "Oren", "Rune", "Schuttgart"}

// adminClanHall answers //ch [action [action2]] <hallId>:
//
//   - //ch <hallId> opens the hall's page;
//   - set gives the free hall to the selected player's clan, unless that
//     clan owns a hall already;
//   - remove frees the hall;
//   - open and close open and close its gates;
//   - teleportto moves the GM to one of its owner restart points;
//   - end ends its auction now;
//   - siege start|end checks its arguments and that the hall is siegable.
//
// Each then opens the hall's page; an unknown action does nothing more.
// Without one to three arguments, or with an id no hall has, the hall list
// opens instead. An id that does not parse is answered with the usage
// alone.
func (l *GameClientLink) adminClanHall(gm *livePlayer, line string) {
	args := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' })
	if len(args) > 0 {
		args = args[1:]
	}
	var param, param2 string
	switch len(args) {
	case 2:
		param = args[0]
	case 3:
		param, param2 = args[0], args[1]
	}
	var (
		hall clanhall.HallView
		ok   bool
	)
	if n := len(args); n >= 1 && n <= 3 {
		id, valid := parseJavaInt(args[n-1])
		if !valid {
			sendText(gm, adminClanHallUsage)
			return
		}
		hall, ok = l.halls.View(id)
	}
	if !ok {
		l.showAdminClanHalls(gm)
		return
	}
	switch param {
	case "set":
		l.adminClanHallSet(gm, hall.ID)
	case "remove":
		if !l.halls.Free(hall.ID) {
			sendText(gm, "This ClanHall is already free.")
		}
	case "open", "close":
		if data, found := l.clanHallData.Get(int(hall.ID)); found {
			l.setHallDoors(data.Gates, param == "open")
		}
	case "teleportto":
		var spawns []location.Location
		if data, found := l.clanHallData.Get(int(hall.ID)); found {
			spawns = data.Spawns[residence.SpawnOwner]
		}
		if len(spawns) == 0 {
			sendText(gm, adminClanHallUsage)
			return
		}
		l.teleportLivePlayer(gm, spawns[rnd.Get(len(spawns))], 0)
	case "end":
		if !l.halls.EndAuction(hall.ID) {
			sendText(gm, "This ClanHall doesn't hold an auction.")
		}
	case "siege":
		l.adminClanHallSiege(gm, hall.ID, param2)
	}
	hall, _ = l.halls.View(hall.ID)
	l.showAdminClanHall(gm, hall)
}

// adminClanHallSet gives hall hallID to the clan of gm's selected player.
// A selection that is no player in a clan is refused as an incorrect
// target.
func (l *GameClientLink) adminClanHallSet(gm *livePlayer, hallID int32) {
	target := adminTargetPlayer(gm, false)
	if target == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}
	cl, ok := l.clanService().ClanOf(target.Character)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}
	switch l.halls.Assign(hallID, cl) {
	case clanhall.AssignHallOwned:
		sendText(gm, "This ClanHall isn't free.")
	case clanhall.AssignClanOwns:
		sendText(gm, "Your target already owns a ClanHall.")
	}
}

// adminClanHallSiege answers //ch siege <action> <hallId>: an action
// missing or all digits gets the usage, a hall that is not siegable says
// so.
func (l *GameClientLink) adminClanHallSiege(gm *livePlayer, hallID int32, action string) {
	switch {
	case action == "" || isDigits(action):
		sendText(gm, "Usage: //ch siege start|end chId.")
	case !l.clanHallData.Siegable(int(hallID)):
		sendText(gm, "This ClanHall isn't siegable.")
	case action == "start" || action == "end":
		// ponytail: #244 owns the hall siege runtime; until it lands a
		// siegable hall's siege cannot be started or ended, and the GM
		// gets the hall's page alone.
		l.log.Warn().Int32("hall_id", hallID).Str("action", action).Msg("admin: //ch siege needs the hall siege runtime")
	}
}

// showAdminClanHall opens hall v's page for gm.
func (l *GameClientLink) showAdminClanHall(gm *livePlayer, v clanhall.HallView) {
	const file = "data/html/admin/clanhall.htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	owner := "None"
	if v.OwnerID != 0 {
		if cl, found := l.clanService().Table().Get(v.OwnerID); found {
			owner = cl.Name()
		}
	}
	var auction strings.Builder
	switch {
	case !v.Auction:
		auction.WriteString("<tr><td>This clanhall doesn't have Auction.</td></tr>")
	case v.Highest == nil:
		auction.WriteString("<tr><td width=150>Bid: 0</td><td width=120>Bidders: " + strconv.Itoa(len(v.Bidders)) +
			"</td></tr><tr><td>End date: " + auctionTime(v.EndDate, auctionDayLayout) + "</td><td>Winner: none</td></tr>")
	default:
		auction.WriteString("<tr><td width=150>Bid: " + groupThousands(int64(v.Highest.Bid)) + "</td><td width=120>Bidders: " +
			strconv.Itoa(len(v.Bidders)) + "</td></tr><tr><td>End date: " + auctionTime(v.EndDate, auctionDayLayout) +
			"</td><td>Winner: " + v.Highest.Name + "</td></tr>")
	}
	// Each placeholder is replaced in turn, the auction rows last. Their
	// placeholder is matched without its closing %, which stays on the
	// page after the rows.
	for _, r := range [][2]string{
		{"%name%", v.Name},
		{"%id%", strconv.Itoa(int(v.ID))},
		{"%size%", strconv.Itoa(v.Size)},
		{"%grade%", strconv.Itoa(v.Grade)},
		{"%lease%", groupThousands(int64(v.Lease))},
		{"%loc%", v.Town},
		{"%desc%", v.Description},
		{"%defaultbid%", groupThousands(int64(v.AuctionMin))},
		{"%owner%", owner},
		{"%paid%", strconv.FormatBool(v.Paid)},
		{"%paiduntil%", auctionTime(v.PaidUntil, auctionDayLayout)},
		{"%auction", auction.String()},
	} {
		page = strings.ReplaceAll(page, r[0], r[1])
	}
	sendValidatedHTML(gm, 0, page, 0)
}

// showAdminClanHalls opens the clan hall list for gm: for each town, its
// halls by id, an owned hall's name marked with a *, and a hall holding an
// auction with its bid count, end date and minimum bid.
func (l *GameClientLink) showAdminClanHalls(gm *livePlayer) {
	const file = "data/html/admin/clanhalls.htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	halls := l.halls.All()
	var b strings.Builder
	for _, town := range adminClanHallTowns {
		b.WriteString(`<font color="LEVEL">` + town + `</font><br><table width=270><tr><td width=130>Clan Hall Name</td><td width=70>End Date</td><td width=70>Min Bid</td></tr>`)
		for _, v := range halls {
			if !strings.EqualFold(v.Town, town) {
				continue
			}
			owned := ""
			if v.OwnerID != 0 {
				owned = "*"
			}
			b.WriteString(`<tr><td><a action="bypass -h admin_ch ` + strconv.Itoa(int(v.ID)) + `">` + v.Name + owned)
			if !v.Auction {
				b.WriteString(`</a></td><td>-</td><td>-</td></tr>`)
				continue
			}
			b.WriteString(" [" + strconv.Itoa(len(v.Bidders)) + "]</a></td><td>" + auctionTime(v.EndDate, auctionDayLayout) +
				"</td><td>" + strconv.Itoa(v.MinBid) + "</td></tr>")
		}
		b.WriteString("</table>")
	}
	page = strings.ReplaceAll(page, "%AGIT_LIST%", b.String())
	sendValidatedHTML(gm, 0, page, 0)
}
