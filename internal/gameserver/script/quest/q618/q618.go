// Package q618 is the quest Into the Flame: Klein, for a character of level
// 60 or more, sends the player to Hilda, who asks for fifty vacualite ore
// from the kookaburras, bandersnatches and grendels, turns it into a
// vacualite, and Klein pays a floating stone for it.
package q618

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q618_IntoTheFlame"

// NPCs.
const (
	klein = 31540
	hilda = 31271
)

// Items.
const (
	vacualiteOre  = 7265
	vacualite     = 7266
	floatingStone = 7267
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Into The Flame",
		QuestID: 618,
		Items:   []int32{vacualiteOre, vacualite},
		Bind: script.Bindings{
			script.EventQuestStart: {klein},
			script.EventTalked:     {klein, hilda},
			script.EventMyDying: {
				21274, 21275, 21276, 21277, 21282, 21283, 21284, 21285, 21290, 21291, 21292, 21293,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, takes the vacualite to Klein for the
// floating stone, sends the player hunting for Hilda, or hands the ore in to
// her for the vacualite.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31540-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31540-05.htm"):
		s.TakeItems(e.Player, vacualite, 1)
		s.GiveItems(e.Player, floatingStone, 1)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "31271-02.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31271-05.htm"):
		st.SetCond(4)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, vacualiteOre, -1)
		s.GiveItems(e.Player, vacualite, 1)
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
		if e.Player.Level() < 60 {
			htmltext = "31540-01.htm"
		} else {
			htmltext = "31540-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case klein:
			if cond == 4 {
				htmltext = "31540-04.htm"
			} else {
				htmltext = "31540-03.htm"
			}
		case hilda:
			switch cond {
			case 1:
				htmltext = "31271-01.htm"
			case 2:
				htmltext = "31271-03.htm"
			case 3:
				htmltext = "31271-04.htm"
			case 4:
				htmltext = "31271-06.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops vacualite ore to a party member at cond 2, and moves the
// one who holds fifty to cond 3.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), vacualiteOre, 1, 50, 500000) {
		st.SetCond(3)
	}
}
