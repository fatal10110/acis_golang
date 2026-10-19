// Package q303 is the quest Collect Arrowheads: Minia pays for ten orcish
// arrowheads taken from the Tumran orcs.
package q303

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q303_CollectArrowheads"

// Item.
const orcishArrowhead = 963

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Collect Arrowheads",
		QuestID: 303,
		Items:   []int32{orcishArrowhead},
		Bind: script.Bindings{
			script.EventQuestStart: {30029}, // Minia
			script.EventTalked:     {30029},
			script.EventMyDying:    {20361},
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
	if strings.EqualFold(e.Name, "30029-03.htm") {
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
		if p.Level() < 10 {
			htmltext = "30029-01.htm"
		} else {
			htmltext = "30029-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30029-04.htm"
		} else {
			htmltext = "30029-05.htm"
			s.TakeItems(p, orcishArrowhead, -1)
			s.RewardItems(p, 57, 1000)
			s.RewardExpAndSp(p, 2000, 0)
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
	if s.DropItems(p, orcishArrowhead, 1, 10, 400000) {
		st.SetCond(2)
	}
}
