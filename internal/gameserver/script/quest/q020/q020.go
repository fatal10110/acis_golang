// Package q020 is the quest Bring Up With Love: Tunatun asks the player to
// raise a wild beast until it yields a jewel of innocence, which he buys
// back for adena.
package q020

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q020_BringUpWithLove"

// jewelOfInnocence is the quest item.
const jewelOfInnocence = 7185

// tunatun is the quest's only NPC.
const tunatun = 31537

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Bring Up With Love",
		QuestID: 20,
		Items:   []int32{jewelOfInnocence},
		Bind: script.Bindings{
			script.EventQuestStart: {tunatun},
			script.EventTalked:     {tunatun},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31537-09.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31537-12.htm"):
		s.TakeItems(p, jewelOfInnocence, -1)
		s.RewardItems(p, 57, 68500)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 65 {
			htmltext = "31537-02.htm"
		} else {
			htmltext = "31537-01.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 2 {
			htmltext = "31537-11.htm"
		} else {
			htmltext = "31537-10.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
