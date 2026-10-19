// Package q272 is the quest Wrath of Ancestors: Livina pays an orc 1500
// adena for fifty grave robbers' heads.
package q272

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q272_WrathOfAncestors"

// Item.
const graveRobbersHead = 1474

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Wrath of Ancestors",
		QuestID: 272,
		Items:   []int32{graveRobbersHead},
		Bind: script.Bindings{
			script.EventQuestStart: {30572}, // Livina
			script.EventTalked:     {30572},
			script.EventMyDying:    {20319, 20320}, // Goblin Grave Robber, Goblin Tomb Raider Leader
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
	if strings.EqualFold(e.Name, "30572-03.htm") {
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
		switch {
		case p.Race() != player.RaceOrc:
			htmltext = "30572-00.htm"
		case p.Level() < 5:
			htmltext = "30572-01.htm"
		default:
			htmltext = "30572-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30572-04.htm"
		} else {
			htmltext = "30572-05.htm"
			s.TakeItems(p, graveRobbersHead, -1)
			s.RewardItems(p, 57, 1500)
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
	if s.DropItemsAlways(p, graveRobbersHead, 1, 50) {
		st.SetCond(2)
	}
}
