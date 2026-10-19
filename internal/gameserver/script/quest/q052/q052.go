// Package q052 is the quest Willie's Special Bait: Willie trades a hundred
// tarlk eyes of the tarlk basilisks for earth fishing lures.
package q052

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q052_WilliesSpecialBait"

// willie is the quest giver.
const willie = 31574

// tarlkBasilisk is the monster that drops the tarlk eye.
const tarlkBasilisk = 20573

// tarlkEye is the quest item.
const tarlkEye = 7623

// earthFishingLure is the reward.
const earthFishingLure = 7612

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Willie's Special Bait",
		QuestID: 52,
		Items:   []int32{tarlkEye},
		Bind: script.Bindings{
			script.EventQuestStart: {willie},
			script.EventTalked:     {willie},
			script.EventMyDying:    {tarlkBasilisk},
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
	case strings.EqualFold(e.Name, "31574-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31574-07.htm"):
		htmltext = "31574-06.htm"
		s.TakeItems(e.Player, tarlkEye, -1)
		s.RewardItems(e.Player, earthFishingLure, 4)
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
		if e.Player.Level() < 48 {
			htmltext = "31574-02.htm"
		} else {
			htmltext = "31574-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(tarlkEye) == 100 {
			htmltext = "31574-04.htm"
		} else {
			htmltext = "31574-05.htm"
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
	if s.DropItems(p, tarlkEye, 1, 100, 500000) {
		st.SetCond(2)
	}
}
