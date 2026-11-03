// Package q110 is the quest To the Primeval Isle: Anton sends an ancient
// book to Marquez on the Primeval Isle, for adena.
package q110

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q110_ToThePrimevalIsle"

// NPCs.
const (
	anton   = 31338
	marquez = 32113
)

// ancientBook is the book Anton sends.
const ancientBook = 8777

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "To the Primeval Isle",
		QuestID: 110,
		Items:   []int32{ancientBook},
		Bind: script.Bindings{
			script.EventQuestStart: {anton},
			script.EventTalked:     {anton, marquez},
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
	case strings.EqualFold(e.Name, "31338-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ancientBook, 1)
	case strings.EqualFold(e.Name, "32113-03.htm") && p.HasItem(ancientBook):
		s.TakeItems(p, ancientBook, 1)
		s.RewardItems(p, 57, 169380)
		s.PlaySound(p, script.SoundFinish)
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
		if e.Player.Level() < 75 {
			htmltext = "31338-00.htm"
		} else {
			htmltext = "31338-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case anton:
			htmltext = "31338-01c.htm"
		case marquez:
			htmltext = "32113-01.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
