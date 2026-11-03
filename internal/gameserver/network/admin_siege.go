package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// adminSiege answers //siege [attack|defend|clear|end|start|list] <alias>:
//
//   - //siege <alias> opens the castle's page;
//   - attack and defend register the clan of the selected player as the
//     player itself asking would (siege.Siege.RegisterAttacker,
//     RegisterDefender), a refusal told to that player; a selection that
//     is no player is refused as an incorrect target, a player in no clan
//     changes nothing;
//   - clear drops every registration, the owner registered again;
//   - end and start end and start the siege now;
//   - list shows the castle's siege window, and nothing else.
//
// Each but list then opens the castle's page; an unknown action is
// answered with the usage first. Without exactly one or two arguments, or
// with an alias no castle has, the castle list opens instead.
//
// The page follows what the siege told the players: the siege posts its
// notices to each player's queue, so the page is posted there too.
func (l *GameClientLink) adminSiege(gm *livePlayer, line string) {
	param, c := l.adminCastleArgs(line)
	var s *siege.Siege
	if c != nil {
		s, _ = l.sieges.Get(c.ID)
	}
	if s == nil {
		l.showAdminCastles(gm)
		return
	}
	switch param {
	case "":
	case "attack", "defend":
		target := adminTargetPlayer(gm, false)
		if target == nil {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
			break
		}
		cl, ok := l.clanService().ClanOf(target.Character)
		if !ok {
			break
		}
		register := s.RegisterAttacker
		if param == "defend" {
			register = s.RegisterDefender
		}
		if refusal, refused := register(cl); refused {
			target.SendFrame(frameSiegeMessage(refusal))
		}
	case "clear":
		s.ClearAll()
	case "end":
		s.End()
	case "start":
		s.Start()
	case "list":
		gm.SendFrame(l.frameSiegeInfo(gm, c))
		return
	default:
		sendText(gm, "Usage: //siege [attack|clear|defend|end|list|start castleName].")
	}
	postLive(gm, func() { l.showAdminCastle(gm, c) })
}

// showAdminCastles opens the castle list for gm: each castle in turn, rows
// alternately on a black background, links to its page and its siege
// window, and where its siege stands.
func (l *GameClientLink) showAdminCastles(gm *livePlayer) {
	const file = "data/html/admin/castles.htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	var rows strings.Builder
	for i, c := range l.castles.All() {
		if i%2 == 0 {
			rows.WriteString("<table width=270 bgcolor=000000><tr>")
		} else {
			rows.WriteString("<table width=270><tr>")
		}
		rows.WriteString(`<td width=100><a action="bypass -h admin_siege ` + c.Alias + `">` + c.Name + `</a></td><td width=130>`)
		rows.WriteString(l.siegeStatus(c))
		rows.WriteString(`</td><td width=40 align=right><a action="bypass admin_siege list ` + c.Alias + `">View</a></td>`)
		rows.WriteString(`</tr></table><img src="L2UI.SquareGray" width=270 height=1>`)
	}
	sendValidatedHTML(gm, 0, strings.ReplaceAll(page, "%castles%", rows.String()), 0)
}

// siegeStatus names where c's siege stands, "" for a castle without one.
func (l *GameClientLink) siegeStatus(c *castle.Castle) string {
	s, ok := l.sieges.Get(c.ID)
	if !ok {
		return ""
	}
	return s.Status().String()
}
