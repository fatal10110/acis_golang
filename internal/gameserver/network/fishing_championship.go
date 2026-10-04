package network

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// measureCatch measures the fish live just caught on lure for the fishing
// championship and tells live its length, then whether it entered the
// running ranking. A disabled championship measures nothing.
func measureCatch(live *livePlayer, champ *fishchamp.Championship, lure int32) {
	catch, ok := champ.NewFish(live.Name, lure)
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCaughtFishS1Length, commons.JavaDouble(catch.Length)))
	if catch.Registered {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageRegisteredInFishSizeRanking))
	}
}

// requestFishRanking answers the fishing window's ranking button with the
// running week's ranking, or, when it is due to be taken anew, with the
// notice that it is being taken. A disabled championship answers nothing,
// as the reference does: the fishing window then shows no ranking button,
// so no client action waits on an answer.
func (l *GameClientLink) requestFishRanking(live *livePlayer) {
	champ := l.fishChamp
	if !champ.Enabled() {
		return
	}
	places, refreshing := champ.Running()
	if refreshing {
		sendFilledHTML(live, 0, l.setPage(fishchamp.PageRefreshing), 0)
		return
	}
	prize, ok := l.fishChampPrizeName()
	if !ok {
		return
	}
	sendFilledHTML(live, 0, champ.FillRunning(l.setPage(fishchamp.PageRunning), places, prize), 0)
}

// fishermanChampionship opens fisherman f's championship page: the last
// week's winners, the prizes and the minutes left in the running week.
func (l *GameClientLink) fishermanChampionship(live *livePlayer, f *npc.Folk) {
	champ := l.fishChamp
	if !champ.Enabled() {
		sendFilledHTML(live, f.ObjectID(), l.setPage(fishchamp.PageDisabled), 0)
		return
	}
	prize, ok := l.fishChampPrizeName()
	if !ok {
		return
	}
	sendFilledHTML(live, f.ObjectID(), champ.FillWinners(l.setPage(fishchamp.PageWinners), f.ObjectID(), prize), 0)
}

// fishermanReward pays live its fishing championship prize at fisherman f:
// the prize item, then the thanks page. A player not among the last week's
// winners is told so; a winner who already claimed, or who placed past
// fifth, gets nothing and no page.
func (l *GameClientLink) fishermanReward(live *livePlayer, f *npc.Folk) {
	champ := l.fishChamp
	switch {
	case !champ.Enabled():
		sendFilledHTML(live, f.ObjectID(), l.setPage(fishchamp.PageDisabled), 0)
		return
	case !champ.IsWinner(live.Name):
		sendFilledHTML(live, f.ObjectID(), l.setPage(fishchamp.PageNotWinner), 0)
		return
	}
	for _, count := range champ.Claim(live.Name) {
		if count <= 0 {
			continue
		}
		live.AddCreatedItem(champ.Config().RewardItemID, int(count), l.nextObjectID)
		sendFilledHTML(live, 0, l.setPage(fishchamp.PageRewarded), 0)
	}
}

// fishChampPrizeName returns the name of the item the championship pays
// its prizes in. A configured item with no template is logged and opens
// no page.
func (l *GameClientLink) fishChampPrizeName() (string, bool) {
	id := l.fishChamp.Config().RewardItemID
	if l.itemTemplates != nil {
		if tmpl, ok := l.itemTemplates.Get(id); ok {
			return tmpl.Name, true
		}
	}
	l.log.Warn().Int32("item_id", id).Msg("fishing championship: prize item has no template")
	return "", false
}
