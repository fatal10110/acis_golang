// Package q009 is the quest Into the City of Humans: Petukai sends an orc
// to Tanapi, then to Tamil, for a mark of traveler and a scroll of escape
// to Giran.
package q009

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q009_IntoTheCityOfHumans"

// NPCs.
const (
	petukai = 30583
	tanapi  = 30571
	tamil   = 30576
)

// Rewards.
const (
	soeGiran       = 7559
	markOfTraveler = 7570
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Into the City of Humans",
		QuestID: 9,
		Bind: script.Bindings{
			script.EventQuestStart: {petukai},
			script.EventTalked:     {petukai, tanapi, tamil},
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
	case strings.EqualFold(e.Name, "30583-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30571-01.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30576-01.htm"):
		s.GiveItems(e.Player, markOfTraveler, 1)
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
		if e.Player.Level() >= 3 && e.Player.Race() == player.RaceOrc {
			htmltext = "30583-00.htm"
		} else {
			htmltext = "30583-00a.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case petukai:
			if cond == 1 {
				htmltext = "30583-01a.htm"
			}
		case tanapi:
			switch cond {
			case 1:
				htmltext = "30571-00.htm"
			case 2:
				htmltext = "30571-01a.htm"
			}
		case tamil:
			if cond == 2 {
				htmltext = "30576-00.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
