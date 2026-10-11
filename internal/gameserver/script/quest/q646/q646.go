// Package q646 is the quest Signs Of Revolt: Torrant wants 180 cursed
// dolls from the ragnas, which drop to one party member at random, and
// pays for them in the reward the player picks.
package q646

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q646_SignsOfRevolt"

// torrant is the quest giver.
const torrant = 32016

// cursedDoll is the quest item.
const cursedDoll = 8087

// reward is one reward choice: an item and its count.
type reward struct{ item, count int32 }

// rewards returns the reward choices, in the order the event numbers them
// from 0.
func rewards() []reward {
	return []reward{
		{1880, 9},
		{1881, 12},
		{1882, 20},
		{57, 21600},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Signs Of Revolt",
		QuestID: 646,
		Items:   []int32{cursedDoll},
		Bind: script.Bindings{
			script.EventQuestStart: {torrant},
			script.EventTalked:     {torrant},
			script.EventMyDying: {
				22029, 22030, 22031, 22032, 22033, 22034, 22035, 22036, 22037, 22038, 22039, 22040,
				22041, 22042, 22043, 22044, 22045, 22047, 22049,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or pays the reward the event numbers. A
// number off the list panics once the dolls are taken, as the reference
// throws there.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32016-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case script.IsDigit(e.Name):
		htmltext = "32016-07.htm"
		s.TakeItems(e.Player, cursedDoll, -1)
		choice, err := strconv.Atoi(e.Name)
		if err != nil {
			panic(err)
		}
		r := rewards()[choice]
		s.GiveItems(e.Player, r.item, r.count)
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
		if e.Player.Level() < 40 {
			htmltext = "32016-02.htm"
		} else {
			htmltext = "32016-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32016-04.htm"
		case 2:
			htmltext = "32016-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops a doll to one party member at random among those at
// cond 1 near the ragna; the 180th moves that member to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), cursedDoll, 1, 180, 750000) {
		st.SetCond(2)
	}
}
