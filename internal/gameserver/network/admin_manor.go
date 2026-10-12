package network

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminManor answers //manor with the manor page: the mode, when it next
// changes, and each castle's running and next period cost in castles.xml
// order.
//
// With the manor off there is no mode change or list to show: the command
// is logged and answered with ActionFailed.
func (l *GameClientLink) adminManor(gm *livePlayer, _ string) {
	m := l.castleManor
	if !m.Enabled() {
		l.log.Warn().Str("command", "admin_manor").Msg("admin: manor is disabled")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	const file = "data/html/admin/manor.htm"
	page, ok := l.html.Get(file)
	if !ok {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	var info strings.Builder
	for _, c := range l.castles.All() {
		info.WriteString("<tr><td width=110>Name:</td><td width=160><font color=008000>" + c.Name + "</font></td></tr>")
		info.WriteString("<tr><td>Current period cost:</td><td><font color=FF9900>" + groupThousands(m.ManorCost(c.ID, false)) + " Adena</font></td></tr>")
		info.WriteString("<tr><td>Next period cost:</td><td><font color=FF9900>" + groupThousands(m.ManorCost(c.ID, true)) + " Adena</font></td></tr>")
		info.WriteString("<tr><td>&nbsp;</td></tr>")
	}
	page = strings.NewReplacer(
		"%status%", m.Status().String(),
		"%change%", m.NextModeChange().Format("02/01 15:04:05"),
		"%castleInfo%", info.String(),
	).Replace(page)
	sendValidatedHTML(gm, 0, page, 0)
}
