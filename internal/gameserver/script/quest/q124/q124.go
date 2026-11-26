// Package q124 is the quest Meeting the Elroki: Marquez sends the character
// through Mushika, Asamah, Karakawei and Mantarasa, whose egg Asamah takes
// for adena.
package q124

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q124_MeetingTheElroki"

// NPCs.
const (
	marquez   = 32113
	mushika   = 32114
	asamah    = 32115
	karakawei = 32117
	mantarasa = 32118
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Meeting the Elroki",
		QuestID: 124,
		Bind: script.Bindings{
			script.EventQuestStart: {marquez},
			script.EventTalked:     {marquez, mushika, asamah, karakawei, mantarasa},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// condPages returns, for each page that only advances the quest, the cond
// it sets, with the middle sound.
func condPages() map[string]int32 {
	return map[string]int32{
		"32113-04.htm": 2,
		"32114-02.htm": 3,
		"32115-04.htm": 4,
		"32117-03.htm": 5,
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
	case strings.EqualFold(e.Name, "32113-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32117-02.htm"):
		if st.Cond() == 4 {
			st.Set("progress", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "32118-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, 8778, 1) // Egg
	default:
		for page, cond := range condPages() {
			if strings.EqualFold(e.Name, page) {
				st.SetCond(cond)
				s.PlaySound(p, script.SoundMiddle)
				break
			}
		}
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
		if p.Level() < 75 {
			htmltext = "32113-01a.htm"
		} else {
			htmltext = "32113-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case marquez:
			if cond == 1 {
				htmltext = "32113-03.htm"
			} else if cond > 1 {
				htmltext = "32113-04a.htm"
			}
		case mushika:
			if cond == 2 {
				htmltext = "32114-01.htm"
			} else if cond > 2 {
				htmltext = "32114-03.htm"
			}
		case asamah:
			if cond == 3 {
				htmltext = "32115-01.htm"
			} else if cond == 6 {
				htmltext = "32115-05.htm"
				s.TakeItems(p, 8778, -1)
				s.RewardItems(p, 57, 71318)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case karakawei:
			if cond == 4 {
				htmltext = "32117-01.htm"
				if st.GetInt("progress") == 1 {
					htmltext = "32117-02.htm"
				}
			} else if cond > 4 {
				htmltext = "32117-04.htm"
			}
		case mantarasa:
			if cond == 5 {
				htmltext = "32118-01.htm"
			} else if cond > 5 {
				htmltext = "32118-03.htm"
			}
		}
	case questlog.StatusCompleted:
		if e.NPC.NpcID() == asamah {
			htmltext = "32115-06.htm"
		} else {
			htmltext = script.AlreadyCompletedMsg()
		}
	}
	return htmltext
}
