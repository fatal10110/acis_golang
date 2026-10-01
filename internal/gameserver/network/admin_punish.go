package network

import (
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminKick answers //kick all|name: every player that is not a game
// master, or the named player (the selected player when none is so named),
// is disconnected with ServerClose.
func (l *GameClientLink) adminKick(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, "Usage : //kick [all|name]")
		return
	}
	if args[0] == "all" {
		if l.world == nil {
			return
		}
		for _, p := range l.world.Players() {
			if live, ok := p.(*livePlayer); ok && !live.access.IsGM {
				live.kickClient()
			}
		}
		return
	}
	target := l.adminNamedPlayer(gm, args[0], false)
	if target == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	target.kickClient()
}
