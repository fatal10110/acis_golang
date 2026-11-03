// Package q261 is the quest Collector's Dream: eight giant spider legs earn
// Alshupes's adena and experience.
package q261

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q261_CollectorsDream"

// Item.
const giantSpiderLeg = 1087

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Collector's Dream",
		QuestID: 261,
		Items:   []int32{giantSpiderLeg},
		Bind: script.Bindings{
			script.EventQuestStart: {30222}, // Alshupes
			script.EventTalked:     {30222},
			script.EventMyDying:    {20308, 20460, 20466},
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
	if strings.EqualFold(e.Name, "30222-03.htm") {
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
		if p.Level() < 15 {
			htmltext = "30222-01.htm"
		} else {
			htmltext = "30222-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 2 {
			htmltext = "30222-05.htm"
			s.TakeItems(p, giantSpiderLeg, -1)
			s.RewardItems(p, 57, 1000)
			s.RewardExpAndSp(p, 2000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "30222-04.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItemsAlways(p, giantSpiderLeg, 1, 8) {
		st.SetCond(2)
	}
}
