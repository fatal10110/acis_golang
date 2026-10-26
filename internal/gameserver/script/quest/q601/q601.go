// Package q601 is the quest Watching Eyes: the Eye of Argos asks a
// character of level 71 or more for the proof of avenger from the
// Disciples, and pays adena and, by chance, an item, items and experience.
package q601

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q601_WatchingEyes"

// eyeOfArgos is the quest giver.
const eyeOfArgos = 31683

// proofOfAvenger is the item the Disciples drop.
const proofOfAvenger = 7188

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Watching Eyes",
		QuestID: 601,
		Items:   []int32{proofOfAvenger},
		Bind: script.Bindings{
			script.EventQuestStart: {eyeOfArgos},
			script.EventTalked:     {eyeOfArgos},
			script.EventMyDying:    {21306, 21308, 21309, 21310, 21311},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// reward returns the item, the adena and the draw bound of the first row
// of the reward table that a draw of random under 100 falls under; item 0
// is a row with no item, and ok is false for a draw no row takes.
func reward(random int) (item, adena int32, ok bool) {
	switch {
	case random < 20:
		return 6699, 90000, true
	case random < 40:
		return 6698, 80000, true
	case random < 50:
		return 6700, 40000, true
	case random < 100:
		return 0, 230000, true
	}
	return 0, 0, false
}

// onAdvEvent starts the quest for a character of level 71, or pays the
// reward row a draw of 100 picks. An item row also pays five of the item
// and experience.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31683-03.htm"):
		if e.Player.Level() < 71 {
			htmltext = "31683-02.htm"
		} else {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		}
	case strings.EqualFold(e.Name, "31683-07.htm"):
		s.TakeItems(e.Player, proofOfAvenger, -1)
		if item, adena, ok := reward(s.Rnd(100)); ok {
			s.RewardItems(e.Player, 57, adena)
			if item != 0 {
				s.GiveItems(e.Player, item, 5)
				s.RewardExpAndSp(e.Player, 120000, 10000)
			}
		}
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
		htmltext = "31683-01.htm"
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			if e.Player.HasItem(proofOfAvenger) {
				htmltext = "31683-05.htm"
			} else {
				htmltext = "31683-04.htm"
			}
		case 2:
			htmltext = "31683-06.htm"
		}
	}
	return htmltext
}

// onMyDying drops a proof to a party member at cond 1, and moves the one
// who holds a hundred to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), proofOfAvenger, 1, 100, 500000) {
		st.SetCond(2)
	}
}
