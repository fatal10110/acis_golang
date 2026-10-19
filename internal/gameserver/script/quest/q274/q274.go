// Package q274 is the quest Skirmish with the Werewolves: Brukurse pays an
// orc holding a necklace of courage or valor for forty Maraku werewolf
// heads, and 600 adena more for each wolfmen totem.
package q274

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q274_SkirmishWithTheWerewolves"

// Needed items.
const (
	necklaceOfValor   = 1507
	necklaceOfCourage = 1506
)

// Items.
const (
	marakuWerewolfHead = 1477
	marakuWolfmenTotem = 1501
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Skirmish with the Werewolves",
		QuestID: 274,
		Items:   []int32{marakuWerewolfHead, marakuWolfmenTotem},
		Bind: script.Bindings{
			script.EventQuestStart: {30569},
			script.EventTalked:     {30569},
			script.EventMyDying:    {20363, 20364},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	htmltext := e.Name
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30569-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		case p.Race() != player.RaceOrc:
			htmltext = "30569-00.htm"
		case p.Level() < 9:
			htmltext = "30569-01.htm"
		case p.HasAtLeastOneItem(necklaceOfCourage, necklaceOfValor):
			htmltext = "30569-02.htm"
		default:
			htmltext = "30569-07.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30569-04.htm"
		} else {
			htmltext = "30569-05.htm"
			amount := 3500 + p.ItemCount(marakuWolfmenTotem)*600
			s.TakeItems(p, marakuWerewolfHead, -1)
			s.TakeItems(p, marakuWolfmenTotem, -1)
			s.RewardItems(p, 57, amount)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItemsAlways(p, marakuWerewolfHead, 1, 40) {
		st.SetCond(2)
	}
	if s.Rnd(100) < 6 {
		s.GiveItems(p, marakuWolfmenTotem, 1)
	}
}
