// Package q053 is the quest Linnaeus' Special Bait: Linnaeus trades a
// hundred crimson drake hearts for flaming fishing lures.
package q053

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q053_LinnaeusSpecialBait"

// linnaeus is the quest giver.
const linnaeus = 31577

// crimsonDrake is the monster that drops the crimson drake heart.
const crimsonDrake = 20670

// crimsonDrakeHeart is the quest item.
const crimsonDrakeHeart = 7624

// flamingFishingLure is the reward.
const flamingFishingLure = 7613

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Linnaues' Special Bait",
		QuestID: 53,
		Items:   []int32{crimsonDrakeHeart},
		Bind: script.Bindings{
			script.EventQuestStart: {linnaeus},
			script.EventTalked:     {linnaeus},
			script.EventMyDying:    {crimsonDrake},
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
	case strings.EqualFold(e.Name, "31577-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31577-07.htm"):
		htmltext = "31577-06.htm"
		s.TakeItems(e.Player, crimsonDrakeHeart, -1)
		s.RewardItems(e.Player, flamingFishingLure, 4)
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
		if e.Player.Level() < 60 {
			htmltext = "31577-02.htm"
		} else {
			htmltext = "31577-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(crimsonDrakeHeart) == 100 {
			htmltext = "31577-04.htm"
		} else {
			htmltext = "31577-05.htm"
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
	if s.DropItems(p, crimsonDrakeHeart, 1, 100, 500000) {
		st.SetCond(2)
	}
}
