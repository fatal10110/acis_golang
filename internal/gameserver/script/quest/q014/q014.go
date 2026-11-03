// Package q014 is the quest Whereabouts of the Archaeologist: Liesel gives a
// player a letter, which the ghost of an adventurer takes for adena.
package q014

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q014_WhereaboutsOfTheArchaeologist"

// NPCs.
const (
	liesel            = 31263
	ghostOfAdventurer = 31538
)

// letter is the quest item.
const letter = 7253

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Whereabouts of the Archaeologist",
		QuestID: 14,
		Items:   []int32{letter},
		Bind: script.Bindings{
			script.EventQuestStart: {liesel},
			script.EventTalked:     {liesel, ghostOfAdventurer},
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
	switch {
	case strings.EqualFold(e.Name, "31263-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, letter, 1)
	case strings.EqualFold(e.Name, "31538-1.htm"):
		s.TakeItems(e.Player, letter, 1)
		s.RewardItems(e.Player, 57, 113228)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 74 {
			htmltext = "31263-1.htm"
		} else {
			htmltext = "31263-0.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case liesel:
			htmltext = "31263-2.htm"
		case ghostOfAdventurer:
			htmltext = "31538-0.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
