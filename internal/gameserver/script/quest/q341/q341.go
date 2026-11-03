// Package q341 is the quest Hunting for Wild Beasts: Pano pays for twenty
// bear skins.
package q341

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q341_HuntingForWildBeasts"

// Item.
const bearSkin = 4259

// chances returns the bear skin drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		20021: 500000, // Red Bear
		20203: 900000, // Dion Grizzly
		20310: 500000, // Brown Bear
		20335: 700000, // Grizzly Bear
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Hunting for Wild Beasts",
		QuestID: 341,
		Items:   []int32{bearSkin},
		Bind: script.Bindings{
			script.EventQuestStart: {30078}, // Pano
			script.EventTalked:     {30078},
			script.EventMyDying:    {20021, 20203, 20310, 20335},
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
	if strings.EqualFold(e.Name, "30078-02.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 20 {
			htmltext = "30078-00.htm"
		} else {
			htmltext = "30078-01.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(bearSkin) < 20 {
			htmltext = "30078-03.htm"
		} else {
			htmltext = "30078-04.htm"
			s.TakeItems(p, bearSkin, -1)
			s.RewardItems(p, 57, 3710)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropItems(p, bearSkin, 1, 20, chances()[e.NPC.NpcID()])
}
