// Package q629 is the quest Clean Up the Swamp of Screams: Pierce, for a
// character of level 66 or more who holds a mercenary certificate of the
// Golden Ram, asks for talons of Stakato that the monsters of the swamp
// drop, and pays Golden Ram coins for a hundred of them.
package q629

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q629_CleanUpTheSwampOfScreams"

// NPC.
const pierce = 31553

// Items.
const (
	talonOfStakato = 7250
	goldenRamCoin  = 7251
)

// chance returns the monster npcID's drop chance of a talon of Stakato.
func chance(npcID int32) int32 {
	switch npcID {
	case 21508:
		return 500000
	case 21509:
		return 431000
	case 21510:
		return 521000
	case 21511:
		return 576000
	case 21512:
		return 746000
	case 21513:
		return 530000
	case 21514:
		return 538000
	case 21515:
		return 545000
	case 21516:
		return 553000
	case 21517:
		return 560000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Clean up the Swamp of Screams",
		QuestID: 629,
		Items:   []int32{talonOfStakato, goldenRamCoin},
		Bind: script.Bindings{
			script.EventQuestStart: {pierce},
			script.EventTalked:     {pierce},
			script.EventMyDying: {
				21508, 21509, 21510, 21511, 21512, 21513, 21514, 21515, 21516, 21517,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest for a character of level 66 or more, trades a
// hundred talons for twenty coins, or ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31553-1.htm"):
		if e.Player.Level() >= 66 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		} else {
			htmltext = "31553-0a.htm"
			st.Exit(true)
		}
	case strings.EqualFold(e.Name, "31553-3.htm"):
		if e.Player.ItemCount(talonOfStakato) >= 100 {
			s.TakeItems(e.Player, talonOfStakato, 100)
			s.GiveItems(e.Player, goldenRamCoin, 20)
		} else {
			htmltext = "31553-3a.htm"
		}
	case strings.EqualFold(e.Name, "31553-5.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// onTalk answers a character that holds neither of the two certificates
// with the page that sends it for one, whatever its state.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if !e.Player.HasAtLeastOneItem(7246, 7247) {
		return "31553-6.htm"
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 66 {
			htmltext = "31553-0a.htm"
		} else {
			htmltext = "31553-0.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(talonOfStakato) >= 100 {
			htmltext = "31553-2.htm"
		} else {
			htmltext = "31553-1a.htm"
		}
	}
	return htmltext
}

// onMyDying drops a talon of Stakato to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), talonOfStakato, 1, 100, chance(e.NPC.NpcID()))
}
