// Package q167 is the quest Dwarven Kinship: Carlon's letter goes to Haprock,
// who either keeps it or sends Norman's letter on to Norman.
package q167

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q167_DwarvenKinship"

// Items.
const (
	carlonLetter = 1076
	normanLetter = 1106
)

// NPCs.
const (
	carlon  = 30350
	norman  = 30210
	haprock = 30255
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dwarven Kinship",
		QuestID: 167,
		Items:   []int32{carlonLetter, normanLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {carlon},
			script.EventTalked:     {carlon, haprock, norman},
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
	case strings.EqualFold(e.Name, "30350-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, carlonLetter, 1)
	case strings.EqualFold(e.Name, "30255-03.htm"):
		st.SetCond(2)
		s.TakeItems(p, carlonLetter, 1)
		s.GiveItems(p, normanLetter, 1)
		s.RewardItems(p, 57, 2000)
	case strings.EqualFold(e.Name, "30255-04.htm"):
		s.TakeItems(p, carlonLetter, 1)
		s.RewardItems(p, 57, 3000)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "30210-02.htm"):
		s.TakeItems(p, normanLetter, 1)
		s.RewardItems(p, 57, 20000)
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
		if e.Player.Level() < 15 {
			htmltext = "30350-02.htm"
		} else {
			htmltext = "30350-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case carlon:
			if cond == 1 {
				htmltext = "30350-05.htm"
			}
		case haprock:
			if cond == 1 {
				htmltext = "30255-01.htm"
			} else if cond == 2 {
				htmltext = "30255-05.htm"
			}
		case norman:
			if cond == 2 {
				htmltext = "30210-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
