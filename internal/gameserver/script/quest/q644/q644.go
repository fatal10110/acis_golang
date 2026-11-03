// Package q644 is the quest Grave Robber Annihilation: Karuda, for a
// character of level 20 or more, asks for a hundred and twenty goods of the
// orc grave robbers and pays the item the page picks.
package q644

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q644_GraveRobberAnnihilation"

// NPC.
const karuda = 32017

// Items.
const orcGraveGoods = 8088

// rewards returns the item and its count the page picks by index.
func rewards() [][]int32 {
	return [][]int32{
		{1865, 30},
		{1867, 40},
		{1872, 40},
		{1871, 30},
		{1870, 30},
		{1869, 30},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Grave Robber Annihilation",
		QuestID: 644,
		Items:   []int32{orcGraveGoods},
		Bind: script.Bindings{
			script.EventQuestStart: {karuda},
			script.EventTalked:     {karuda},
			script.EventMyDying:    {22003, 22004, 22005, 22006, 22008},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or pays the reward whose index the page sent
// and ends it; the client can only send a link of the page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32017-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case script.IsDigit(e.Name):
		htmltext = "32017-04.htm"
		s.TakeItems(e.Player, orcGraveGoods, -1)

		index, err := strconv.Atoi(e.Name)
		if err != nil {
			panic(err)
		}
		reward := rewards()[index]
		s.RewardItems(e.Player, reward[0], reward[1])

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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 20 {
			htmltext = "32017-06.htm"
		} else {
			htmltext = "32017-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32017-05.htm"
		case 2:
			htmltext = "32017-07.htm"
		}
	}
	return htmltext
}

// onMyDying drops goods to a party member at cond 1, and moves the one who
// then holds a hundred and twenty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), orcGraveGoods, 1, 120, 500000) {
		st.SetCond(2)
	}
}
