// Package q369 is the quest Collector of Jewels: Nell collects the flare and
// freezing shards of the Ice and Fire elementals.
package q369

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q369_CollectorOfJewels"

// nell is the quest giver.
const nell = 30376

// Quest items.
const (
	flareShard    = 5882
	freezingShard = 5883
)

// adena is the money item.
const adena = 57

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Collector of Jewels",
		QuestID: 369,
		Items:   []int32{flareShard, freezingShard},
		Bind: script.Bindings{
			script.EventQuestStart: {nell},
			script.EventTalked:     {nell},
			script.EventMyDying:    {20609, 20612, 20749, 20616, 20619, 20747},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30376-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30376-07.htm"):
		s.PlaySound(e.Player, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30376-08.htm"):
		st.Exit(true)
		s.PlaySound(e.Player, script.SoundFinish)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 25 {
			htmltext = "30376-01.htm"
		} else {
			htmltext = "30376-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		flare := p.ItemCount(flareShard)
		freezing := p.ItemCount(freezingShard)
		switch {
		case cond == 1:
			htmltext = "30376-04.htm"
		case cond == 2 && flare >= 50 && freezing >= 50:
			htmltext = "30376-05.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, flareShard, -1)
			s.TakeItems(p, freezingShard, -1)
			s.RewardItems(p, adena, 12500)
		case cond == 3:
			htmltext = "30376-09.htm"
		case cond == 4 && flare >= 200 && freezing >= 200:
			htmltext = "30376-10.htm"
			s.TakeItems(p, flareShard, -1)
			s.TakeItems(p, freezingShard, -1)
			s.RewardItems(p, adena, 63500)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p := st.Player()
	item, chance := drop(e.NPC.NpcID())
	other := int32(flareShard)
	if item == flareShard {
		other = freezingShard
	}
	switch st.Cond() {
	case 1:
		if s.DropItems(p, item, 1, 50, chance) && p.ItemCount(other) >= 50 {
			st.SetCond(2)
		}
	case 3:
		if s.DropItems(p, item, 1, 200, chance) && p.ItemCount(other) >= 200 {
			st.SetCond(4)
		}
	}
}

// drop is the shard and the drop chance of a monster.
func drop(npcID int32) (item, chance int32) {
	switch npcID {
	case 20609:
		return flareShard, 630000
	case 20612:
		return flareShard, 770000
	case 20749:
		return flareShard, 850000
	case 20616:
		return freezingShard, 600000
	case 20619:
		return freezingShard, 730000
	default:
		return freezingShard, 850000
	}
}
