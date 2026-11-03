// Package q264 is the quest Keen Claws: fifty wolf claws earn one of Payne's
// rewards, picked by a seventeen-sided roll.
package q264

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q264_KeenClaws"

// Item.
const wolfClaw = 1367

// Rewards.
const (
	leatherSandals = 36
	woodenHelmet   = 43
	stockings      = 462
	healingPotion  = 1061
	shortGloves    = 48
	clothShoes     = 35
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Keen Claws",
		QuestID: 264,
		Items:   []int32{wolfClaw},
		Bind: script.Bindings{
			script.EventQuestStart: {30136}, // Payne
			script.EventTalked:     {30136},
			script.EventMyDying:    {20003, 20456}, // Goblin, Wolf
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
	if strings.EqualFold(e.Name, "30136-03.htm") {
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
		if p.Level() < 3 {
			htmltext = "30136-01.htm"
		} else {
			htmltext = "30136-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(wolfClaw) < 50 {
			htmltext = "30136-04.htm"
		} else {
			htmltext = "30136-05.htm"
			s.TakeItems(p, wolfClaw, -1)
			n := s.Rnd(17)
			switch {
			case n == 0:
				s.GiveItems(p, woodenHelmet, 1)
				s.PlaySound(p, script.SoundJackpot)
			case n < 2:
				s.GiveItems(p, 57, 1000)
			case n < 5:
				s.GiveItems(p, leatherSandals, 1)
			case n < 8:
				s.GiveItems(p, stockings, 1)
				s.GiveItems(p, 57, 50)
			case n < 11:
				s.GiveItems(p, healingPotion, 1)
			case n < 14:
				s.GiveItems(p, shortGloves, 1)
			default:
				s.GiveItems(p, clothShoes, 1)
			}
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
	if e.NPC.NpcID() == 20003 {
		count := int32(4)
		if s.RndBool() {
			count = 2
		}
		if s.DropItems(p, wolfClaw, count, 50, 500000) {
			st.SetCond(2)
		}
		return
	}
	count := int32(2)
	if s.Rnd(5) < 4 {
		count = 1
	}
	if s.DropItemsAlways(p, wolfClaw, count, 50) {
		st.SetCond(2)
	}
}
