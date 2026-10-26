// Package q614 is the quest Slay the Enemy Commander: Ashas Varka Durai asks
// a Varka ally of level 4 or more for the head of Tayr, the Ketra raid boss,
// and pays for it with the Feather of Wisdom. Every ally of the killer's
// party near Tayr gets a head.
package q614

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q614_SlayTheEnemyCommander"

// ashas is the quest giver, Ashas Varka Durai.
const ashas = 31377

// tayr is the raid boss whose head the quest asks for.
const tayr = 25302

// Items.
const (
	headOfTayr      = 7241
	featherOfWisdom = 7230
	varkaAlliance4  = 7224
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Slay the enemy commander!",
		QuestID: 614,
		Items:   []int32{headOfTayr},
		Bind: script.Bindings{
			script.EventQuestStart: {ashas},
			script.EventTalked:     {ashas},
			script.EventMyDying:    {tayr},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the feather. Without
// the head the quest goes back to cond 1.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31377-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31377-07.htm"):
		if e.Player.HasItem(headOfTayr) {
			s.TakeItems(e.Player, headOfTayr, -1)
			s.GiveItems(e.Player, featherOfWisdom, 1)
			s.RewardExpAndSp(e.Player, 10000, 0)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31377-06.htm"
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		}
	}
	return htmltext
}

// onTalk offers the quest to a level 75 Varka ally of level 4 or more who
// holds the level 4 alliance mark and no feather yet.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case e.Player.Level() < 75:
			htmltext = "31377-03.htm"
		case e.Player.VarkaKetraAlliance() <= -4 && e.Player.HasItem(varkaAlliance4) && !e.Player.HasItem(featherOfWisdom):
			htmltext = "31377-01.htm"
		default:
			htmltext = "31377-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(headOfTayr) {
			htmltext = "31377-05.htm"
		} else {
			htmltext = "31377-06.htm"
		}
	}
	return htmltext
}

// onMyDying gives a head to every party member at cond 1 near Tayr who is a
// Varka ally of level 4 or more and holds the alliance mark, and moves each
// to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(1)) {
		pm := st.Player()
		if pm.VarkaKetraAlliance() <= -4 && pm.HasItem(varkaAlliance4) {
			st.SetCond(2)
			s.PlaySound(pm, script.SoundMiddle)
			s.GiveItems(pm, headOfTayr, 1)
		}
	}
}
