// Package q017 is the quest Light and Darkness: the hierarch gives a player
// four bloods of saint, one for each altar of saints, visited in order, for
// experience.
package q017

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q017_LightAndDarkness"

// bloodOfSaint is the quest item.
const bloodOfSaint = 7168

// NPCs.
const (
	hierarch    = 31517
	saintAltar1 = 31508
	saintAltar2 = 31509
	saintAltar3 = 31510
	saintAltar4 = 31511
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Light and Darkness",
		QuestID: 17,
		Items:   []int32{bloodOfSaint},
		Bind: script.Bindings{
			script.EventQuestStart: {hierarch},
			script.EventTalked:     {hierarch, saintAltar1, saintAltar2, saintAltar3, saintAltar4},
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
	case strings.EqualFold(e.Name, "31517-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, bloodOfSaint, 4)
	case strings.EqualFold(e.Name, "31508-02.htm"):
		if e.Player.HasItem(bloodOfSaint) {
			st.SetCond(2)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, bloodOfSaint, 1)
		} else {
			htmltext = "31508-03.htm"
		}
	case strings.EqualFold(e.Name, "31509-02.htm"):
		if e.Player.HasItem(bloodOfSaint) {
			st.SetCond(3)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, bloodOfSaint, 1)
		} else {
			htmltext = "31509-03.htm"
		}
	case strings.EqualFold(e.Name, "31510-02.htm"):
		if e.Player.HasItem(bloodOfSaint) {
			st.SetCond(4)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, bloodOfSaint, 1)
		} else {
			htmltext = "31510-03.htm"
		}
	case strings.EqualFold(e.Name, "31511-02.htm"):
		if e.Player.HasItem(bloodOfSaint) {
			st.SetCond(5)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, bloodOfSaint, 1)
		} else {
			htmltext = "31511-03.htm"
		}
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
		if e.Player.Level() < 61 {
			htmltext = "31517-03.htm"
		} else {
			htmltext = "31517-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case hierarch:
			if cond == 5 {
				htmltext = "31517-07.htm"
				s.RewardExpAndSp(e.Player, 105527, 0)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			} else if e.Player.HasItem(bloodOfSaint) {
				htmltext = "31517-05.htm"
			} else {
				htmltext = "31517-06.htm"
				st.Exit(true)
			}
		case saintAltar1:
			if cond == 1 {
				htmltext = "31508-01.htm"
			} else if cond > 1 {
				htmltext = "31508-04.htm"
			}
		case saintAltar2:
			if cond == 2 {
				htmltext = "31509-01.htm"
			} else if cond > 2 {
				htmltext = "31509-04.htm"
			}
		case saintAltar3:
			if cond == 3 {
				htmltext = "31510-01.htm"
			} else if cond > 3 {
				htmltext = "31510-04.htm"
			}
		case saintAltar4:
			if cond == 4 {
				htmltext = "31511-01.htm"
			} else if cond > 4 {
				htmltext = "31511-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
