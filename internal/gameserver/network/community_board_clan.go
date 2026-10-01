package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// clanBoardLevel is the clan level a clan's board pages open at.
const clanBoardLevel = 2

// clanBoardAccess is the access the clan management page shows for the
// clan's announcement and bulletin boards.
// ponytail: those boards are forums (#3201); every clan forum is made with
// read access and nothing changes it, so the page shows that access until
// the forums exist.
const clanBoardAccess = "Read access"

// boardClan runs a clan board command: _bbsclan shows the player's clan,
// or the clan list without one; _bbsclan;<clan|home|mail|management>;<n>
// shows the list page n or clan n's home, mail form or management form;
// _bbsclan;notice[;<true|false>] shows the player's clan notice settings,
// first switching the login notice on or off. Any other command shows
// nothing, as the management page's permission links do.
func (l *GameClientLink) boardClan(live *livePlayer, command string) {
	if strings.EqualFold(command, "_bbsclan") {
		if cl, ok := l.clanService().ClanOf(live.Character); ok {
			l.showClanHome(live, cl.ID())
		} else {
			l.showClanList(live, 1)
		}
		return
	}
	tokens := bbs.Tokens(command, ";")
	if len(tokens) < 2 {
		return
	}
	action := tokens[1]
	if strings.EqualFold(action, "notice") {
		if len(tokens) > 2 {
			if cl, ok := l.clanService().ClanOf(live.Character); ok {
				l.clanService().EnableNotice(cl, strings.EqualFold(tokens[2], "true"))
			}
		}
		l.showClanNotice(live, l.boardClanID(live))
		return
	}
	if len(tokens) < 3 {
		return
	}
	n, err := strconv.ParseInt(tokens[2], 10, 32)
	if err != nil {
		return
	}
	switch {
	case strings.EqualFold(action, "clan"):
		l.showClanList(live, int(n))
	case strings.EqualFold(action, "home"):
		l.showClanHome(live, int32(n))
	case strings.EqualFold(action, "mail"):
		l.showClanMailForm(live, int32(n))
	case strings.EqualFold(action, "management"):
		l.showClanManagement(live, int32(n))
	}
}

// boardClanWrite submits a clan board form: intro (clan id, introduction),
// notice (notice text in the fourth argument) or mail (clan id, subject
// and message in the fourth and fifth arguments). The introduction and the
// clan mail take any member of the clan the form names; the notice takes
// any member of a clan.
func (l *GameClientLink) boardClanWrite(live *livePlayer, args [5]string) {
	switch {
	case strings.EqualFold(args[0], "intro"):
		cl, ok := l.boardFormClan(live, args[1])
		if !ok {
			return
		}
		l.clanService().SetIntroduction(cl, args[2])
		l.showClanManagement(live, cl.ID())
	case args[0] == "notice":
		if cl, ok := l.clanService().ClanOf(live.Character); ok {
			l.clanService().SetNotice(cl, args[3])
			l.showClanNotice(live, cl.ID())
		}
	case strings.EqualFold(args[0], "mail"):
		cl, ok := l.boardFormClan(live, args[1])
		if !ok {
			return
		}
		members := cl.MembersInTableOrder()
		names := make([]string, len(members))
		for i, m := range members {
			names[i] = m.Name
		}
		l.sendMail(live, strings.Join(names, ";"), args[3], args[4])
		l.showClanHome(live, cl.ID())
	default:
		l.sendBoard(live, bbs.NotImplemented(args[0]))
	}
}

// boardFormClan returns the player's clan when a clan form names it by id.
func (l *GameClientLink) boardFormClan(live *livePlayer, id string) (*clan.Clan, bool) {
	n, err := strconv.ParseInt(id, 10, 32)
	if err != nil || int32(n) != l.boardClanID(live) {
		return nil, false
	}
	return l.clanService().Table().Get(int32(n))
}

// boardClanID is the id of the player's clan, 0 without one.
func (l *GameClientLink) boardClanID(live *livePlayer) int32 {
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		return cl.ID()
	}
	return 0
}

// clanCard is cl as the clan board shows it.
func clanCard(cl *clan.Clan) bbs.ClanCard {
	info := cl.Info()
	return bbs.ClanCard{
		ID: info.ID, Name: info.Name, LeaderName: info.LeaderName, Level: info.Level,
		Members: cl.MembersCount(), AllyID: info.AllyID, AllyName: info.AllyName,
		Introduction: cl.Introduction(),
	}
}

// showClanList shows the clan list from page index.
func (l *GameClientLink) showClanList(live *livePlayer, index int) {
	page, ok := l.boardPage(bbs.ClanListPage)
	if !ok {
		return
	}
	clans := l.clanService().Table().Clans()
	cards := make([]bbs.ClanCard, len(clans))
	for i, cl := range clans {
		cards[i] = clanCard(cl)
	}
	l.sendBoard(live, bbs.RenderClanList(page, cards, index, l.boardClanID(live)))
}

// clanBoardOpen reports whether cl's board pages open; one that does not
// answers that the clan has no board and shows the clan list.
func (l *GameClientLink) clanBoardOpen(live *livePlayer, cl *clan.Clan) bool {
	if cl.Level() >= clanBoardLevel {
		return true
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoCommunityBoardInClan))
	l.showClanList(live, 1)
	return false
}

// clanLeaderPage reports whether live leads the clan id; one that does not
// is told only the leader may, and shown the clan list.
func (l *GameClientLink) clanLeaderPage(live *livePlayer, cl *clan.Clan) bool {
	if l.boardClanID(live) == cl.ID() && cl.IsLeader(live.ObjectID()) {
		return true
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderEnabled))
	l.showClanList(live, 1)
	return false
}

// showClanHome shows clan id's home page: the visitor's, the member's or
// the leader's.
func (l *GameClientLink) showClanHome(live *livePlayer, id int32) {
	cl, ok := l.clanService().Table().Get(id)
	if !ok || !l.clanBoardOpen(live, cl) {
		return
	}
	name := bbs.ClanHomePage
	switch {
	case l.boardClanID(live) != id:
	case cl.IsLeader(live.ObjectID()):
		name = bbs.ClanHomeLeaderPage
	default:
		name = bbs.ClanHomeMemberPage
	}
	if page, ok := l.boardPage(name); ok {
		l.sendBoard(live, bbs.RenderClanHome(page, clanCard(cl)))
	}
}

// showClanMailForm opens the clan mail form, to the leader of clan id.
func (l *GameClientLink) showClanMailForm(live *livePlayer, id int32) {
	cl, ok := l.clanService().Table().Get(id)
	if !ok || !l.clanLeaderPage(live, cl) {
		return
	}
	if page, ok := l.boardPage(bbs.ClanMailPage); ok {
		l.sendBoard(live, bbs.RenderClanMail(page, id, cl.Name()))
	}
}

// showClanManagement opens the clan management form, to the leader of clan
// id, filled with its introduction. A clan below the board level has no
// boards to show the access of, and shows nothing.
func (l *GameClientLink) showClanManagement(live *livePlayer, id int32) {
	cl, ok := l.clanService().Table().Get(id)
	if !ok || !l.clanLeaderPage(live, cl) || cl.Level() < clanBoardLevel {
		return
	}
	if page, ok := l.boardPage(bbs.ClanManagementPage); ok {
		l.sendBoardEdit(live, bbs.RenderClanManagement(page, id, clanBoardAccess), cl.Introduction(), "", "")
	}
}

// showClanNotice opens the notice settings of clan id, the player's own,
// filled with its notice.
func (l *GameClientLink) showClanNotice(live *livePlayer, id int32) {
	cl, ok := l.clanService().Table().Get(id)
	if !ok || l.boardClanID(live) != id || !l.clanBoardOpen(live, cl) {
		return
	}
	page, ok := l.boardPage(bbs.ClanNoticePage)
	if !ok {
		return
	}
	notice, enabled := cl.Notice()
	l.sendBoardEdit(live, bbs.RenderClanNotice(page, id, enabled), notice, "", "")
}
