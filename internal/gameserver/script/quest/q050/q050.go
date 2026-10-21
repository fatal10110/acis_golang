// Package q050 is the quest Lanosco's Special Bait: Lanosco trades a
// hundred essences of wind of the singing winds for wind fishing lures.
package q050

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q050_LanoscosSpecialBait"

// lanosco is the quest giver.
const lanosco = 31570

// singingWind is the monster that drops the essence of wind.
const singingWind = 21026

// essenceOfWind is the quest item.
const essenceOfWind = 7621

// windFishingLure is the reward.
const windFishingLure = 7610

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Lanosco's Special Bait",
		QuestID: 50,
		Items:   []int32{essenceOfWind},
		Bind: script.Bindings{
			script.EventQuestStart: {lanosco},
			script.EventTalked:     {lanosco},
			script.EventMyDying:    {singingWind},
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
	case strings.EqualFold(e.Name, "31570-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31570-07.htm"):
		htmltext = "31570-06.htm"
		s.TakeItems(e.Player, essenceOfWind, -1)
		s.RewardItems(e.Player, windFishingLure, 4)
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
		if e.Player.Level() < 27 {
			htmltext = "31570-02.htm"
		} else {
			htmltext = "31570-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(essenceOfWind) == 100 {
			htmltext = "31570-04.htm"
		} else {
			htmltext = "31570-05.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, essenceOfWind, 1, 100, 500000) {
		st.SetCond(2)
	}
}
