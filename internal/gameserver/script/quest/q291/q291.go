// Package q291 is the quest Revenge of the Redbonnet: Maryse Redbonnet
// trades forty black wolf pelts for one of grandma's keepsakes, picked by a
// hundred-sided roll.
package q291

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q291_RevengeOfTheRedbonnet"

// Quest item.
const blackWolfPelt = 1482

// Rewards.
const (
	scrollOfEscape  = 736
	grandmaPearl    = 1502
	grandmaMirror   = 1503
	grandmaNecklace = 1504
	grandmaHairpin  = 1505
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Revenge of the Redbonnet",
		QuestID: 291,
		Items:   []int32{blackWolfPelt},
		Bind: script.Bindings{
			script.EventQuestStart: {30553}, // Maryse Redbonnet
			script.EventTalked:     {30553},
			script.EventMyDying:    {20317},
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
	if strings.EqualFold(e.Name, "30553-03.htm") {
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
		if p.Level() < 4 {
			htmltext = "30553-01.htm"
		} else {
			htmltext = "30553-02.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30553-04.htm"
		case 2:
			htmltext = "30553-05.htm"
			s.TakeItems(p, blackWolfPelt, -1)
			random := s.Rnd(100)
			switch {
			case random < 3:
				s.RewardItems(p, grandmaPearl, 1)
			case random < 21:
				s.RewardItems(p, grandmaMirror, 1)
			case random < 46:
				s.RewardItems(p, grandmaNecklace, 1)
			default:
				s.RewardItems(p, scrollOfEscape, 1)
				s.RewardItems(p, grandmaHairpin, 1)
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
	if s.DropItemsAlways(p, blackWolfPelt, 1, 40) {
		st.SetCond(2)
	}
}
