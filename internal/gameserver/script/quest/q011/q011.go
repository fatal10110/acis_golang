// Package q011 is the quest Secret Meeting With Ketra Orcs: Cadmon sends a
// player to Leon for a box of munitions, which Wahkan takes for experience.
package q011

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q011_SecretMeetingWithKetraOrcs"

// NPCs.
const (
	cadmon = 31296
	leon   = 31256
	wahkan = 31371
)

// munitionsBox is the quest item.
const munitionsBox = 7231

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Secret Meeting With Ketra Orcs",
		QuestID: 11,
		Items:   []int32{munitionsBox},
		Bind: script.Bindings{
			script.EventQuestStart: {cadmon},
			script.EventTalked:     {cadmon, leon, wahkan},
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
	case strings.EqualFold(e.Name, "31256-02.htm"):
		s.GiveItems(e.Player, munitionsBox, 1)
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31371-02.htm"):
		s.TakeItems(e.Player, munitionsBox, 1)
		s.RewardExpAndSp(e.Player, 79787, 0)
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
		case leon:
			switch cond {
			case 1:
				htmltext = "31256-01.htm"
			case 2:
				htmltext = "31256-03.htm"
			}
		case wahkan:
			if cond == 2 {
				htmltext = "31371-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
