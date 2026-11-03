// Package q645 is the quest Ghosts of Batur: Karuda, for a character of level
// 23 or more, collects the cursed grave goods of the Batur ghosts and pays the
// item the page picks for them.
package q645

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q645_GhostsOfBatur"

// NPC.
const karuda = 32017

// Items.
const cursedGraveGoods = 8089

// rewards returns the item and its count the page picks by index.
func rewards() [][]int32 {
	return [][]int32{
		{1878, 18},
		{1879, 7},
		{1880, 4},
		{1881, 6},
		{1882, 10},
		{1883, 2},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Ghosts Of Batur",
		QuestID: 645,
		Bind: script.Bindings{
			script.EventQuestStart: {karuda},
			script.EventTalked:     {karuda},
			script.EventMyDying:    {22007, 22009, 22010, 22011, 22012, 22013, 22014, 22015, 22016},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or pays the reward whose index the page sent
// for the goods and ends the quest; the client can only send a link of the
// page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32017-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case script.IsDigit(e.Name):
		htmltext = "32017-07.htm"
		s.TakeItems(e.Player, cursedGraveGoods, -1)

		index, err := strconv.Atoi(e.Name)
		if err != nil {
			panic(err)
		}
		reward := rewards()[index]
		s.GiveItems(e.Player, reward[0], reward[1])

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
		if e.Player.Level() < 23 {
			htmltext = "32017-02.htm"
		} else {
			htmltext = "32017-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32017-04.htm"
		case 2:
			htmltext = "32017-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops cursed grave goods to a party member at cond 1, and moves
// the one who then holds one hundred and eighty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), cursedGraveGoods, 1, 180, 750000) {
		st.SetCond(2)
	}
}
