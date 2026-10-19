// Package q018 is the quest Meeting with the Golden Ram: Donal sends the
// player to Daisy for a supply box, which Abercrombie takes in exchange for
// adena and experience.
package q018

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q018_MeetingWithTheGoldenRam"

// supplyBox is the quest item.
const supplyBox = 7245

// NPCs.
const (
	donal       = 31314
	daisy       = 31315
	abercrombie = 31555
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Meeting with the Golden Ram",
		QuestID: 18,
		Items:   []int32{supplyBox},
		Bind: script.Bindings{
			script.EventQuestStart: {donal},
			script.EventTalked:     {donal, daisy, abercrombie},
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
	case strings.EqualFold(e.Name, "31314-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31315-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, supplyBox, 1)
	case strings.EqualFold(e.Name, "31555-02.htm"):
		s.TakeItems(p, supplyBox, 1)
		s.RewardItems(p, 57, 15000)
		s.RewardExpAndSp(p, 50000, 0)
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
		if e.Player.Level() < 66 {
			htmltext = "31314-02.htm"
		} else {
			htmltext = "31314-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case donal:
			htmltext = "31314-04.htm"
		case daisy:
			if cond == 1 {
				htmltext = "31315-01.htm"
			} else if cond == 2 {
				htmltext = "31315-03.htm"
			}
		case abercrombie:
			if cond == 2 {
				htmltext = "31555-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
