package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// festivalGuideBypass runs a festival guide's or witch's "Festival <val>"
// command, or its "FestivalDesc <val>" one when desc is set, for live at f:
// its messages, then its page, then the page's ActionFailed, then the
// party expulsion it asks for. It reports false when the command aborted,
// so that nothing more is sent.
func (l *GameClientLink) festivalGuideBypass(live *livePlayer, f *npc.Folk, command string, val int, desc bool) bool {
	guide, ok := festival.GuideOf(f.NpcID())
	if !ok || l.festival == nil {
		return false
	}
	var reply festival.GuideReply
	if desc {
		reply = l.festival.Description(guide, val)
	} else {
		// "Festival 9" takes live out of its arena inside the call.
		live.RunOnOwnQueue(func() {
			reply = l.festival.Command(guide, l.festivalTalker(live), f.ObjectID(), val, command)
		})
	}
	if reply.Aborted {
		return false
	}
	if reply.Message != "" {
		sendText(live, reply.Message)
	}
	if reply.NotEnoughItems {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	}
	if reply.Contribution > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageContribScoreIncreasedS1, int32(reply.Contribution)))
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
	if reply.Expel && l.parties != nil {
		l.applyOwnPartyNotices(live, l.parties.Leave(live, party.Expelled))
	}
	return true
}

// festivalTalker is what a festival guide reads of live: its name, party,
// whether its cabal is the one the Seven Signs scores put ahead, and how it
// pays.
func (l *GameClientLink) festivalTalker(live *livePlayer) festival.GuideTalker {
	t := festival.GuideTalker{
		ObjectID: live.ObjectID(),
		Name:     live.CharacterName(),
		Pay: func(itemID int32, count int) bool {
			return destroyItemByItemID(live, itemID, count)
		},
		TakeOfferings: func() (int, bool, bool) {
			inv := live.Inventory()
			if inv == nil {
				return 0, false, false
			}
			inst := inv.ItemByTemplateID(festival.BloodOfferingID)
			if inst == nil {
				return 0, false, false
			}
			count := inst.Snapshot().Count
			return count, true, inv.DestroyItem(inst, count) != nil
		},
	}
	if l.sevenSigns != nil {
		t.Winning = l.sevenSigns.PlayerCabal(live.ObjectID()) == l.sevenSigns.WinningCabal()
	}
	if l.parties != nil {
		if v, ok := l.parties.View(live.ObjectID()); ok {
			t.Party = &festival.GuideParty{Leader: v.Leader.ObjectID(), Members: festivalMembers(v.Members), Level: v.Level}
		}
	}
	return t
}

// festivalMembers is members as the festival keeps them.
func festivalMembers(members []*livePlayer) []festival.Member {
	out := make([]festival.Member, len(members))
	for i, m := range members {
		out[i] = festival.Member{ObjectID: m.ObjectID(), Name: m.CharacterName()}
	}
	return out
}
