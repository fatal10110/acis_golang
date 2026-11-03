// Package q051 is the quest O'Fulle's Special Bait: O'Fulle trades a
// hundred lost baits of the fettered souls for icy air lures.
package q051

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q051_OFullesSpecialBait"

// ofulle is the quest giver.
const ofulle = 31572

// fetteredSoul is the monster that drops the lost bait.
const fetteredSoul = 20552

// lostBait is the quest item.
const lostBait = 7622

// icyAirLure is the reward.
const icyAirLure = 7611

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "O'Fulle's Special Bait",
		QuestID: 51,
		Items:   []int32{lostBait},
		Bind: script.Bindings{
			script.EventQuestStart: {ofulle},
			script.EventTalked:     {ofulle},
			script.EventMyDying:    {fetteredSoul},
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
	case strings.EqualFold(e.Name, "31572-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31572-07.htm"):
		htmltext = "31572-06.htm"
		s.TakeItems(e.Player, lostBait, -1)
		s.RewardItems(e.Player, icyAirLure, 4)
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
		if e.Player.Level() < 36 {
			htmltext = "31572-02.htm"
		} else {
			htmltext = "31572-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(lostBait) == 100 {
			htmltext = "31572-04.htm"
		} else {
			htmltext = "31572-05.htm"
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
	if s.DropItemsAlways(p, lostBait, 1, 100) {
		st.SetCond(2)
	}
}
