// Package q007 is the quest A Trip Begins: Mirabel sends an elf to Ariel,
// whose recommendation goes to Asterios, for a mark of traveler and a
// scroll of escape to Giran.
package q007

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q007_ATripBegins"

// NPCs.
const (
	mirabel  = 30146
	ariel    = 30148
	asterios = 30154
)

// arielReco is the quest item.
const arielReco = 7572

// Rewards.
const (
	soeGiran     = 7559
	markTraveler = 7570
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Trip Begins",
		QuestID: 7,
		Items:   []int32{arielReco},
		Bind: script.Bindings{
			script.EventQuestStart: {mirabel},
			script.EventTalked:     {mirabel, ariel, asterios},
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
	case strings.EqualFold(e.Name, "30146-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30148-02.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, arielReco, 1)
	case strings.EqualFold(e.Name, "30154-02.htm"):
		st.SetCond(3)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, arielReco, 1)
	case strings.EqualFold(e.Name, "30146-06.htm"):
		s.GiveItems(e.Player, markTraveler, 1)
		s.RewardItems(e.Player, soeGiran, 1)
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
		switch {
		case e.Player.Race() != player.RaceElf:
			htmltext = "30146-01.htm"
		case e.Player.Level() < 3:
			htmltext = "30146-01a.htm"
		default:
			htmltext = "30146-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case mirabel:
			switch cond {
			case 1, 2:
				htmltext = "30146-04.htm"
			case 3:
				htmltext = "30146-05.htm"
			}
		case ariel:
			switch cond {
			case 1:
				htmltext = "30148-01.htm"
			case 2:
				htmltext = "30148-03.htm"
			}
		case asterios:
			switch cond {
			case 2:
				htmltext = "30154-01.htm"
			case 3:
				htmltext = "30154-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
