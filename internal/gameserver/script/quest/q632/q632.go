// Package q632 is the quest Necromancer's Request: the Mysterious Wizard, for
// a character of level 63 or more, asks for two hundred vampire hearts and
// pays adena for each batch, as often as the character brings it. Zombie
// brains drop too, and no one asks for them.
package q632

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q632_NecromancersRequest"

// NPC.
const mysteriousWizard = 31522

// Items.
const (
	vampireHeart = 7542
	zombieBrain  = 7543
)

// vampires returns the monsters that drop vampire hearts.
func vampires() []int32 {
	return []int32{21568, 21573, 21582, 21585, 21586, 21587, 21588, 21589, 21590, 21591, 21592, 21593, 21594, 21595}
}

// undeads returns the monsters that drop zombie brains.
func undeads() []int32 {
	return []int32{21547, 21548, 21549, 21551, 21552, 21555, 21556, 21562, 21571, 21576, 21577, 21579}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Necromancer's Request",
		QuestID: 632,
		Items:   []int32{vampireHeart, zombieBrain},
		Bind: script.Bindings{
			script.EventQuestStart: {mysteriousWizard},
			script.EventTalked:     {mysteriousWizard},
			script.EventMyDying:    append(vampires(), undeads()...),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for two hundred vampire hearts, or
// ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31522-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31522-06.htm"):
		if e.Player.ItemCount(vampireHeart) >= 200 {
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, vampireHeart, -1)
			s.RewardItems(e.Player, 57, 120000)
		} else {
			htmltext = "31522-09.htm"
		}
	case strings.EqualFold(e.Name, "31522-08.htm"):
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
		if e.Player.Level() < 63 {
			htmltext = "31522-01.htm"
		} else {
			htmltext = "31522-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(vampireHeart) >= 200 {
			htmltext = "31522-05.htm"
		} else {
			htmltext = "31522-04.htm"
		}
	}
	return htmltext
}

// onMyDying drops a zombie brain, or at cond 1 a vampire heart, to a party
// member who has the quest; the one who then holds two hundred hearts moves
// to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	for _, id := range undeads() {
		if id == e.NPC.NpcID() {
			s.DropItems(st.Player(), zombieBrain, 1, 0, 330000)
			return
		}
	}
	if st.Cond() == 1 && s.DropItems(st.Player(), vampireHeart, 1, 200, 500000) {
		st.SetCond(2)
	}
}
