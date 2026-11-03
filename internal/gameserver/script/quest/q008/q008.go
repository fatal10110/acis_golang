// Package q008 is the quest An Adventure Begins: Jasmine sends a dark elf
// to Roselyn, whose note goes to Harne, for a mark of traveler and a scroll
// of escape to Giran.
package q008

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q008_AnAdventureBegins"

// NPCs.
const (
	jasmine = 30134
	roselyn = 30355
	harne   = 30144
)

// roselynNote is the quest item.
const roselynNote = 7573

// Rewards.
const (
	soeGiran     = 7559
	markTraveler = 7570
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "An Adventure Begins",
		QuestID: 8,
		Items:   []int32{roselynNote},
		Bind: script.Bindings{
			script.EventQuestStart: {jasmine},
			script.EventTalked:     {jasmine, roselyn, harne},
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
	case strings.EqualFold(e.Name, "30134-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30355-02.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, roselynNote, 1)
	case strings.EqualFold(e.Name, "30144-02.htm"):
		st.SetCond(3)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, roselynNote, 1)
	case strings.EqualFold(e.Name, "30134-06.htm"):
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
		if e.Player.Level() >= 3 && e.Player.Race() == player.RaceDarkElf {
			htmltext = "30134-02.htm"
		} else {
			htmltext = "30134-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case jasmine:
			switch cond {
			case 1, 2:
				htmltext = "30134-04.htm"
			case 3:
				htmltext = "30134-05.htm"
			}
		case roselyn:
			switch cond {
			case 1:
				htmltext = "30355-01.htm"
			case 2:
				htmltext = "30355-03.htm"
			}
		case harne:
			switch cond {
			case 2:
				htmltext = "30144-01.htm"
			case 3:
				htmltext = "30144-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
