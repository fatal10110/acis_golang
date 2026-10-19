// Package q258 is the quest Bring Wolf Pelts: forty wolf pelts earn one of
// Lector's armor pieces, picked by a sixteen-sided roll.
package q258

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q258_BringWolfPelts"

// Item.
const wolfPelt = 702

// Rewards.
const (
	cottonShirt        = 390
	leatherPants       = 29
	leatherShirt       = 22
	shortLeatherGloves = 1119
	tunic              = 426
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Bring Wolf Pelts",
		QuestID: 258,
		Items:   []int32{wolfPelt},
		Bind: script.Bindings{
			script.EventQuestStart: {30001}, // Lector
			script.EventTalked:     {30001},
			script.EventMyDying:    {20120, 20442}, // Wolf, Elder Wolf
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
	if strings.EqualFold(e.Name, "30001-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		if p.Level() < 3 {
			htmltext = "30001-01.htm"
		} else {
			htmltext = "30001-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(wolfPelt) < 40 {
			htmltext = "30001-05.htm"
		} else {
			s.TakeItems(p, wolfPelt, -1)
			randomNumber := s.Rnd(16)
			// The reward is a sixteen-sided roll.
			switch {
			case randomNumber == 0:
				s.GiveItems(p, cottonShirt, 1)
			case randomNumber < 6:
				s.GiveItems(p, leatherPants, 1)
			case randomNumber < 9:
				s.GiveItems(p, leatherShirt, 1)
			case randomNumber < 13:
				s.GiveItems(p, shortLeatherGloves, 1)
			default:
				s.GiveItems(p, tunic, 1)
			}
			htmltext = "30001-06.htm"
			if randomNumber == 0 {
				s.PlaySound(p, script.SoundJackpot)
			} else {
				s.PlaySound(p, script.SoundFinish)
			}
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
	if s.DropItemsAlways(p, wolfPelt, 1, 40) {
		st.SetCond(2)
	}
}
