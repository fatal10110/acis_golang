// Package q156 is the quest Millennium Love: Lilith's letter goes to
// Baenedes, who either answers with Theon's diary for Lilith or keeps the
// letter and ends the quest himself.
package q156

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q156_MillenniumLove"

// Items.
const (
	lilithLetter = 1022
	theonDiary   = 1023
)

// NPCs.
const (
	lilith   = 30368
	baenedes = 30369
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Millennium Love",
		QuestID: 156,
		Items:   []int32{lilithLetter, theonDiary},
		Bind: script.Bindings{
			script.EventQuestStart: {lilith},
			script.EventTalked:     {lilith, baenedes},
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
	case strings.EqualFold(e.Name, "30368-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, lilithLetter, 1)
	case strings.EqualFold(e.Name, "30369-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, lilithLetter, 1)
		s.GiveItems(p, theonDiary, 1)
	case strings.EqualFold(e.Name, "30369-03.htm"):
		s.TakeItems(p, lilithLetter, 1)
		s.RewardExpAndSp(p, 3000, 0)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 15 {
			htmltext = "30368-00.htm"
		} else {
			htmltext = "30368-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case lilith:
			if p.HasItem(lilithLetter) {
				htmltext = "30368-05.htm"
			} else if p.HasItem(theonDiary) {
				htmltext = "30368-06.htm"
				s.TakeItems(p, theonDiary, 1)
				s.GiveItems(p, 5250, 1)
				s.RewardExpAndSp(p, 3000, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case baenedes:
			if p.HasItem(lilithLetter) {
				htmltext = "30369-01.htm"
			} else if p.HasItem(theonDiary) {
				htmltext = "30369-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
