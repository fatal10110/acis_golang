// Package q640 is the quest The Zero Hour: Kahman, for a character of level
// 66 or more, collects the fangs of the spiked stakatos and pays the item the
// page picks for its price in fangs.
package q640

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q640_TheZeroHour"

// completedQuest is the quest Kahman wants done before he talks.
const completedQuest = "Q109_InSearchOfTheNest"

// NPC.
const kahman = 31554

// Items.
const fangOfStakato = 8085

// rewards returns the price in fangs, the item and its count the page picks
// by index.
func rewards() [][]int32 {
	return [][]int32{
		{12, 4042, 1},
		{6, 4043, 1},
		{6, 4044, 1},
		{81, 1887, 10},
		{33, 1888, 5},
		{30, 1889, 10},
		{150, 5550, 10},
		{131, 1890, 10},
		{123, 1893, 5},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Zero Hour",
		QuestID: 640,
		Items:   []int32{fangOfStakato},
		Bind: script.Bindings{
			script.EventQuestStart: {kahman},
			script.EventTalked:     {kahman},
			script.EventMyDying: {
				22105, 22106, 22107, 22108, 22109, 22110, 22111, 22113, 22114, 22115, 22116, 22117,
				22118, 22119, 22121,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, sells the reward whose index the page sent
// for its price in fangs, or ends the quest; the client can only send a link
// of the page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31554-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31554-05.htm"):
		if !e.Player.HasItem(fangOfStakato) {
			htmltext = "31554-06.htm"
		}
	case strings.EqualFold(e.Name, "31554-08.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	case script.IsDigit(e.Name):
		index, err := strconv.Atoi(e.Name)
		if err != nil {
			panic(err)
		}
		reward := rewards()[index]
		if e.Player.ItemCount(fangOfStakato) >= reward[0] {
			htmltext = "31554-09.htm"
			s.TakeItems(e.Player, fangOfStakato, reward[0])
			s.RewardItems(e.Player, reward[1], reward[2])
		} else {
			htmltext = "31554-06.htm"
		}
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
		if e.Player.Level() < 66 {
			htmltext = "31554-00.htm"
		} else {
			st2 := s.QuestState(e.Player, completedQuest)
			if st2 != nil && st2.IsCompleted() {
				htmltext = "31554-01.htm"
			} else {
				htmltext = "31554-10.htm"
			}
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(fangOfStakato) {
			htmltext = "31554-04.htm"
		} else {
			htmltext = "31554-03.htm"
		}
	}
	return htmltext
}

// onMyDying drops a fang to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItemsAlways(st.Player(), fangOfStakato, 1, 0)
}
