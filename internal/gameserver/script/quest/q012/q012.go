// Package q012 is the quest Secret Meeting With Varka Silenos: Cadmon sends a
// player to Helmut for a box of munitions, which Naran Ashanuk takes for experience.
package q012

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q012_SecretMeetingWithVarkaSilenos"

// NPCs.
const (
	cadmon       = 31296
	helmut       = 31258
	naranAshanuk = 31378
)

// munitionsBox is the quest item.
const munitionsBox = 7232

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Secret Meeting With Varka Silenos",
		QuestID: 12,
		Items:   []int32{munitionsBox},
		Bind: script.Bindings{
			script.EventQuestStart: {cadmon},
			script.EventTalked:     {cadmon, helmut, naranAshanuk},
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
	case strings.EqualFold(e.Name, "31296-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31258-02.htm"):
		s.GiveItems(e.Player, munitionsBox, 1)
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31378-02.htm"):
		s.TakeItems(e.Player, munitionsBox, 1)
		s.RewardExpAndSp(e.Player, 79761, 0)
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
			htmltext = "31296-02.htm"
		} else {
			htmltext = "31296-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case cadmon:
			if cond == 1 {
				htmltext = "31296-04.htm"
			}
		case helmut:
			switch cond {
			case 1:
				htmltext = "31258-01.htm"
			case 2:
				htmltext = "31258-03.htm"
			}
		case naranAshanuk:
			if cond == 2 {
				htmltext = "31378-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
