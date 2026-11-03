// Package q010 is the quest Into the World: Balanki sends a dwarf to Reed
// with a necklace for Gerald, then back, for a scroll of escape to Giran and
// a mark of traveler.
package q010

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q010_IntoTheWorld"

// NPCs.
const (
	reed    = 30520
	balanki = 30533
	gerald  = 30650
)

// veryExpensiveNecklace is the quest item.
const veryExpensiveNecklace = 7574

// Rewards.
const (
	soeGiran     = 7559
	markTraveler = 7570
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Into the World",
		QuestID: 10,
		Items:   []int32{veryExpensiveNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {balanki},
			script.EventTalked:     {balanki, reed, gerald},
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
	case strings.EqualFold(e.Name, "30533-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30520-02.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, veryExpensiveNecklace, 1)
	case strings.EqualFold(e.Name, "30650-02.htm"):
		st.SetCond(3)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, veryExpensiveNecklace, 1)
	case strings.EqualFold(e.Name, "30520-04.htm"):
		st.SetCond(4)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30533-05.htm"):
		s.GiveItems(e.Player, soeGiran, 1)
		s.RewardItems(e.Player, markTraveler, 1)
		s.PlaySound(e.Player, script.SoundFinish)
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
		if e.Player.Level() >= 3 && e.Player.Race() == player.RaceDwarf {
			htmltext = "30533-01.htm"
		} else {
			htmltext = "30533-01a.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case balanki:
			if cond < 4 {
				htmltext = "30533-03.htm"
			} else if cond == 4 {
				htmltext = "30533-04.htm"
			}
		case reed:
			switch cond {
			case 1:
				htmltext = "30520-01.htm"
			case 2:
				htmltext = "30520-02a.htm"
			case 3:
				htmltext = "30520-03.htm"
			case 4:
				htmltext = "30520-04a.htm"
			}
		case gerald:
			if cond == 2 {
				htmltext = "30650-01.htm"
			} else if cond > 2 {
				htmltext = "30650-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
