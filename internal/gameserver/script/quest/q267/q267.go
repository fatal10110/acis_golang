// Package q267 is the quest Wrath of Verdure: Bremec pays an elf one silvery
// leaf for each goblin club.
package q267

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q267_WrathOfVerdure"

// Item.
const goblinClub = 1335

// Reward.
const silveryLeaf = 1340

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Wrath of Verdure",
		QuestID: 267,
		Items:   []int32{goblinClub},
		Bind: script.Bindings{
			script.EventQuestStart: {31853}, // Bremec
			script.EventTalked:     {31853},
			script.EventMyDying:    {20325}, // Goblin
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31853-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31853-06.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "31853-00.htm"
		case p.Level() < 4:
			htmltext = "31853-01.htm"
		default:
			htmltext = "31853-02.htm"
		}
	case questlog.StatusStarted:
		if count := p.ItemCount(goblinClub); count > 0 {
			htmltext = "31853-05.htm"
			s.TakeItems(p, goblinClub, -1)
			s.RewardItems(p, silveryLeaf, count)
		} else {
			htmltext = "31853-04.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropItems(p, goblinClub, 1, 0, 500000)
}
