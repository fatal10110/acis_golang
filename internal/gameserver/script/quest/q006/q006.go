// Package q006 is the quest Step into the Future: Roxxy sends a human to
// Baulro, whose letter goes to Sir Collin Windawood in Giran, for a mark of
// traveler and a scroll of escape to Giran.
package q006

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q006_StepIntoTheFuture"

// NPCs.
const (
	roxxy     = 30006
	baulro    = 30033
	sirCollin = 30311
)

// baulroLetter is the quest item.
const baulroLetter = 7571

// Rewards.
const (
	soeGiran     = 7559
	markTraveler = 7570
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Step into the Future",
		QuestID: 6,
		Items:   []int32{baulroLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {roxxy},
			script.EventTalked:     {roxxy, baulro, sirCollin},
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
	case strings.EqualFold(e.Name, "30006-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30033-02.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, baulroLetter, 1)
	case strings.EqualFold(e.Name, "30311-02.htm"):
		if e.Player.HasItem(baulroLetter) {
			st.SetCond(3)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, baulroLetter, 1)
		} else {
			htmltext = "30311-03.htm"
		}
	case strings.EqualFold(e.Name, "30006-06.htm"):
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
		if e.Player.Race() != player.RaceHuman || e.Player.Level() < 3 {
			htmltext = "30006-01.htm"
		} else {
			htmltext = "30006-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case roxxy:
			switch cond {
			case 1, 2:
				htmltext = "30006-04.htm"
			case 3:
				htmltext = "30006-05.htm"
			}
		case baulro:
			switch cond {
			case 1:
				htmltext = "30033-01.htm"
			case 2:
				htmltext = "30033-03.htm"
			default:
				htmltext = "30033-04.htm"
			}
		case sirCollin:
			switch cond {
			case 2:
				htmltext = "30311-01.htm"
			case 3:
				htmltext = "30311-03a.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
