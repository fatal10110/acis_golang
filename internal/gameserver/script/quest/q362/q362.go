// Package q362 is the quest Bard's Mandolin: Swan sends the player around
// the village to borrow a flute for his mandolin.
package q362

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q362_BardsMandolin"

// Quest items.
const (
	swanFlute  = 4316
	swanLetter = 4317
)

// Npcs.
const (
	swan    = 30957
	nanarin = 30956
	galion  = 30958
	woodrow = 30837
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Bard's Mandolin",
		QuestID: 362,
		Items:   []int32{swanFlute, swanLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {swan},
			script.EventTalked:     {swan, nanarin, galion, woodrow},
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
	case strings.EqualFold(e.Name, "30957-3.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30957-7.htm") || strings.EqualFold(e.Name, "30957-8.htm"):
		s.RewardItems(e.Player, 57, 10000)
		s.GiveItems(e.Player, 4410, 1)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		if p.Level() < 15 {
			htmltext = "30957-2.htm"
		} else {
			htmltext = "30957-1.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case swan:
			switch {
			case cond == 1 || cond == 2:
				htmltext = "30957-4.htm"
			case cond == 3:
				htmltext = "30957-5.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, swanLetter, 1)
			case cond == 4:
				htmltext = "30957-5a.htm"
			case cond == 5:
				htmltext = "30957-6.htm"
			}
		case woodrow:
			switch {
			case cond == 1:
				htmltext = "30837-1.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 2:
				htmltext = "30837-2.htm"
			case cond > 2:
				htmltext = "30837-3.htm"
			}
		case galion:
			switch {
			case cond == 2:
				htmltext = "30958-1.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, swanFlute, 1)
			case cond > 2:
				htmltext = "30958-2.htm"
			}
		case nanarin:
			switch cond {
			case 4:
				htmltext = "30956-1.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, swanFlute, 1)
				s.TakeItems(p, swanLetter, 1)
			case 5:
				htmltext = "30956-2.htm"
			}
		}
	}
	return htmltext
}
