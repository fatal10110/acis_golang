package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// festivalGuideBypass runs a festival guide's or witch's "Festival <val>"
// command, or its "FestivalDesc <val>" one when desc is set, for live at f:
// its message, then its page, then the page's ActionFailed. A command of
// the festival run, which is not in place yet, is logged. It reports false
// when the command aborted, so that nothing more is sent.
func (l *GameClientLink) festivalGuideBypass(live *livePlayer, f *npc.Folk, command string, val int, desc bool) bool {
	guide, ok := festival.GuideOf(f.NpcID())
	if !ok || l.festival == nil {
		return false
	}
	var reply festival.GuideReply
	if desc {
		reply = l.festival.Description(guide, val)
	} else {
		reply = l.festival.Command(guide, l.festivalTalker(live), f.ObjectID(), val)
	}
	if reply.Aborted {
		return false
	}
	if reply.Unported {
		l.log.Debug().Int("npc_id", f.NpcID()).Str("command", command).Msg("bypass: festival run command not modeled")
		return true
	}
	if reply.Message != "" {
		sendText(live, reply.Message)
	}
	switch {
	case reply.HTML != "":
		sendFilledHTML(live, f.ObjectID(), serverpackets.NpcHtmlBody(reply.HTML), 0)
	case reply.Page != "":
		page := strings.ReplaceAll(l.setPage("data/html/seven_signs/festival/"+reply.Page), "%objectId%", strconv.Itoa(int(f.ObjectID())))
		for i := 0; i+1 < len(reply.Fill); i += 2 {
			page = strings.ReplaceAll(page, reply.Fill[i], reply.Fill[i+1])
		}
		sendFilledHTML(live, f.ObjectID(), page, 0)
	}
	if reply.Release {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	return true
}

// festivalTalker is what a festival guide reads of live: its name, and
// whether its cabal is the one the Seven Signs scores put ahead.
func (l *GameClientLink) festivalTalker(live *livePlayer) festival.GuideTalker {
	t := festival.GuideTalker{Name: live.CharacterName()}
	if l.sevenSigns != nil {
		t.Winning = l.sevenSigns.PlayerCabal(live.ObjectID()) == l.sevenSigns.WinningCabal()
	}
	return t
}
