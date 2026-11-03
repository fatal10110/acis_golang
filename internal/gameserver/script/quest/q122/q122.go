// Package q122 is the quest Ominous News: Moira sends the character to
// Karuda, who pays it in adena.
package q122

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q122_OminousNews"

// NPCs.
const (
	moira  = 31979
	karuda = 32017
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Ominous News",
		QuestID: 122,
		Bind: script.Bindings{
			script.EventQuestStart: {moira},
			script.EventTalked:     {moira, karuda},
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
	case strings.EqualFold(e.Name, "31979-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32017-02.htm"):
		s.RewardItems(p, 57, 1695)
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
		if e.Player.Level() < 20 {
			htmltext = "31979-01.htm"
		} else {
			htmltext = "31979-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case moira:
			htmltext = "31979-03.htm"
		case karuda:
			htmltext = "32017-01.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
