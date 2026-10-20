// Package q121 is the quest Pavel the Giant: Newyear sends the character
// to Yumi, who rewards it with experience.
package q121

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q121_PavelTheGiant"

// NPCs.
const (
	newyear = 31961
	yumi    = 32041
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Pavel the Giant",
		QuestID: 121,
		Bind: script.Bindings{
			script.EventQuestStart: {newyear},
			script.EventTalked:     {newyear, yumi},
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
	case strings.EqualFold(e.Name, "31961-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32041-2.htm"):
		s.RewardExpAndSp(p, 10000, 0)
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
		if e.Player.Level() < 46 {
			htmltext = "31961-1a.htm"
		} else {
			htmltext = "31961-1.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case newyear:
			htmltext = "31961-2a.htm"
		case yumi:
			htmltext = "32041-1.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
