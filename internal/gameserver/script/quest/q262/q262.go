// Package q262 is the quest Trade with the Ivory Tower: ten fungus sacs earn
// Vollodos's adena.
package q262

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q262_TradeWithTheIvoryTower"

// Item.
const fungusSac = 707

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trade with the Ivory Tower",
		QuestID: 262,
		Items:   []int32{fungusSac},
		Bind: script.Bindings{
			script.EventQuestStart: {30137}, // Vollodos
			script.EventTalked:     {30137},
			script.EventMyDying:    {20400, 20007},
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
	if strings.EqualFold(e.Name, "30137-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		if p.Level() < 8 {
			htmltext = "30137-01.htm"
		} else {
			htmltext = "30137-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(fungusSac) < 10 {
			htmltext = "30137-04.htm"
		} else {
			htmltext = "30137-05.htm"
			s.TakeItems(p, fungusSac, -1)
			s.RewardItems(p, 57, 3000)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	chance := int32(300000)
	if e.NPC.NpcID() == 20400 {
		chance = 400000
	}
	if s.DropItems(p, fungusSac, 1, 10, chance) {
		st.SetCond(2)
	}
}
