package network

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminMainPage answers //admin [panel]: the main panel, panel 2, 3 or 4
// for the game, effects or server panel, or a named <name>_menu.htm page
// that exists.
func (l *GameClientLink) adminMainPage(gm *livePlayer, line string) {
	name := "main"
	if args := handleradmin.Args(line); len(args) > 0 {
		param := args[0]
		if handleradmin.IsDigits(param) {
			mode, ok := commons.ParseInt32(param)
			if !ok {
				// A panel number past the int range opens nothing.
				return
			}
			switch mode {
			case 2:
				name = "game"
			case 3:
				name = "effects"
			case 4:
				name = "server"
			}
		} else if _, ok := l.html.Get("data/html/admin/" + param + "_menu.htm"); ok {
			name = param
		}
	}
	l.sendAdminFile(gm, name+"_menu.htm")
}

// adminLink answers //link <file>: that admin page, the main panel without
// a file.
func (l *GameClientLink) adminLink(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.sendAdminFile(gm, "main_menu.htm")
		return
	}
	l.sendAdminFile(gm, args[0])
}

// adminSystemMessage answers //msg <id>: the system message with that id,
// sent without parameters.
func (l *GameClientLink) adminSystemMessage(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, "Usage: //msg sysMsgId")
		return
	}
	id, ok := commons.ParseInt32(args[0])
	if !ok {
		sendText(gm, "Usage: //msg sysMsgId")
		return
	}
	gm.SendFrame(serverpackets.FrameSystemMessage(int(id)))
}
