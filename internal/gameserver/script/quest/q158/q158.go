// Package q158 is the quest Seed of Evil: the player slays the Nerkas who
// guards a clay tablet, and Biotin trades the tablet for a scroll of enchant
// armor (D grade).
package q158

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q158_SeedOfEvil"

// Item.
const clayTablet = 1025

// Reward.
const enchantArmorD = 956

// NpcStrings.
const (
	sayChallenged = 15804
	sayDying      = 15805
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Seed of Evil",
		QuestID: 158,
		Items:   []int32{clayTablet},
		Bind: script.Bindings{
			script.EventQuestStart: {30031}, // Biotin
			script.EventTalked:     {30031},
			script.EventAttacked:   {27016},
			script.EventMyDying:    {27016},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30031-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 21 {
			htmltext = "30031-02.htm"
		} else {
			htmltext = "30031-03.htm"
		}
	case questlog.StatusStarted:
		if !p.HasItem(clayTablet) {
			htmltext = "30031-05.htm"
		} else {
			htmltext = "30031-06.htm"
			s.TakeItems(p, clayTablet, 1)
			s.GiveItems(p, enchantArmorD, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onAttacked(_ *script.Script, e script.Attacked) {
	if e.NPC.IsScriptValue(1) {
		return
	}
	e.NPC.Say(script.NpcString(sayChallenged))
	e.NPC.SetScriptValue(1)
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	st.SetCond(2)
	s.PlaySound(p, script.SoundMiddle)
	s.GiveItems(p, clayTablet, 1)
	e.NPC.Say(script.NpcString(sayDying))
}
