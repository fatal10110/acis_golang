// Package q164 is the quest Blood Fiend: Creamees pays adena for the skull
// of Kirunak.
package q164

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q164_BloodFiend"

// Item.
const kirunakSkull = 1044

// NpcStrings.
const (
	sayChallenged = 16404
	sayDying      = 16405
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Blood Fiend",
		QuestID: 164,
		Items:   []int32{kirunakSkull},
		Bind: script.Bindings{
			script.EventQuestStart: {30149},
			script.EventTalked:     {30149},
			script.EventAttacked:   {27021},
			script.EventMyDying:    {27021},
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
	if strings.EqualFold(e.Name, "30149-04.htm") {
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
		case p.Race() == player.RaceDarkElf:
			htmltext = "30149-00.htm"
		case p.Level() < 21:
			htmltext = "30149-02.htm"
		default:
			htmltext = "30149-03.htm"
		}
	case questlog.StatusStarted:
		if p.HasItem(kirunakSkull) {
			htmltext = "30149-06.htm"
			s.TakeItems(p, kirunakSkull, 1)
			s.RewardItems(p, 57, 42130)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30149-05.htm"
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
	s.GiveItems(p, kirunakSkull, 1)
	e.NPC.Say(script.NpcString(sayDying))
}
