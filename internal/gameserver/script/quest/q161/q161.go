// Package q161 is the quest Fruit of the Mothertree: an elf brings
// Andellia's letter to Thalia and carries the Mother Tree's fruit back.
package q161

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q161_FruitOfTheMotherTree"

// NPCs.
const (
	andellia = 30362
	thalia   = 30371
)

// Items.
const (
	andelliaLetter  = 1036
	mothertreeFruit = 1037
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Fruit of the Mothertree",
		QuestID: 161,
		Items:   []int32{andelliaLetter, mothertreeFruit},
		Bind: script.Bindings{
			script.EventQuestStart: {andellia},
			script.EventTalked:     {andellia, thalia},
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
	if strings.EqualFold(e.Name, "30362-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, andelliaLetter, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "30362-00.htm"
		case p.Level() < 3:
			htmltext = "30362-02.htm"
		default:
			htmltext = "30362-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case andellia:
			switch cond {
			case 1:
				htmltext = "30362-05.htm"
			case 2:
				htmltext = "30362-06.htm"
				s.TakeItems(p, mothertreeFruit, 1)
				s.RewardItems(p, 57, 1000)
				s.RewardExpAndSp(p, 1000, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case thalia:
			switch cond {
			case 1:
				htmltext = "30371-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, andelliaLetter, 1)
				s.GiveItems(p, mothertreeFruit, 1)
			case 2:
				htmltext = "30371-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
