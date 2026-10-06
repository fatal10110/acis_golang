package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// talkToHostile is a hostile NPC's answer to a player's interact in reach.
// Only a talking NPC (a town guard or a friendly monster) answers: its talk
// animation for everyone watching it; the NPC becomes the player's last
// quest NPC, and a script holding its first talk answers instead of it;
// otherwise its chat window opens, then ActionFailed. Any other hostile NPC
// answers nothing; the interact's think has already released the client.
func (l *GameClientLink) talkToHostile(live *livePlayer, h *npc.Hostile) {
	if !h.Talks() {
		return
	}
	if id, ok := h.TalkAnimation(); ok {
		l.broadcastNPCFrame(h, func() wire.Frame { return serverpackets.FrameSocialAction(h.ObjectID(), id) })
	}
	if l.talkThroughScripts(live, h) {
		return
	}
	file := h.ChatPage(func(file string) bool {
		if l.html == nil {
			return false
		}
		_, ok := l.html.Get(file)
		return ok
	})
	page := "<html><body>My html is missing:<br>" + file + "</body></html>"
	if l.html != nil {
		page = l.setPage(file)
	}
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(h.ObjectID())))
	sendFilledHTML(live, h.ObjectID(), page, 0)
	live.SendFrame(serverpackets.FrameActionFailed())
}
