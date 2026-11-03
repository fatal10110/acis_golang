// Package q170 is the quest Dangerous Seduction: Vellior pays adena for the
// nightmare crystal of Merkenis.
package q170

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q170_DangerousSeduction"

// Item.
const nightmareCrystal = 1046

// NpcStrings.
const (
	sayChallenged = 17004
	sayDying      = 17005
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dangerous Seduction",
		QuestID: 170,
		Items:   []int32{nightmareCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {30305},
			script.EventTalked:     {30305},
			script.EventAttacked:   {27022},
			script.EventMyDying:    {27022},
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
	if strings.EqualFold(e.Name, "30305-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30305-00.htm"
		case p.Level() < 21:
			htmltext = "30305-02.htm"
		default:
			htmltext = "30305-03.htm"
		}
	case questlog.StatusStarted:
		if p.HasItem(nightmareCrystal) {
			htmltext = "30305-06.htm"
			s.TakeItems(p, nightmareCrystal, -1)
			s.RewardItems(p, 57, 102680)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30305-05.htm"
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
	s.GiveItems(p, nightmareCrystal, 1)
	e.NPC.Say(script.NpcString(sayDying))
}
