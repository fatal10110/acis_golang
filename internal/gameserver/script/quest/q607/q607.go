// Package q607 is the quest Prove Your Courage: Kadun Zu Ketra asks a
// Ketra ally of level 3 or more for the head of Shadith, the Varka raid
// boss, and pays for it with the Totem of Valor. Every ally of the killer's
// party near Shadith gets a head.
package q607

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q607_ProveYourCourage"

// kadun is the quest giver, Kadun Zu Ketra.
const kadun = 31370

// shadith is the raid boss whose head the quest asks for.
const shadith = 25309

// Items.
const (
	headOfShadith  = 7235
	totemOfValor   = 7219
	ketraAlliance3 = 7213
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Prove your courage!",
		QuestID: 607,
		Items:   []int32{headOfShadith},
		Bind: script.Bindings{
			script.EventQuestStart: {kadun},
			script.EventTalked:     {kadun},
			script.EventMyDying:    {shadith},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem. Without
// the head the quest goes back to cond 1. The start link checks nothing,
// as in the reference.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31370-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31370-07.htm"):
		if e.Player.HasItem(headOfShadith) {
			s.TakeItems(e.Player, headOfShadith, -1)
			s.GiveItems(e.Player, totemOfValor, 1)
			s.RewardExpAndSp(e.Player, 10000, 0)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31370-06.htm"
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		}
	}
	return htmltext
}

// onTalk offers the quest to a level 75 Ketra ally of level 3 or more who
// holds the level 3 alliance mark and no totem yet.
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
			htmltext = "31370-03.htm"
		case e.Player.VarkaKetraAlliance() >= 3 && e.Player.HasItem(ketraAlliance3) && !e.Player.HasItem(totemOfValor):
			htmltext = "31370-01.htm"
		default:
			htmltext = "31370-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(headOfShadith) {
			htmltext = "31370-05.htm"
		} else {
			htmltext = "31370-06.htm"
		}
	}
	return htmltext
}

// onMyDying gives a head to every party member at cond 1 near Shadith who
// is a Ketra ally of level 3 or more and holds the alliance mark, and moves
// each to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(1)) {
		pm := st.Player()
		if pm.VarkaKetraAlliance() >= 3 && pm.HasItem(ketraAlliance3) {
			st.SetCond(2)
			s.PlaySound(pm, script.SoundMiddle)
			s.GiveItems(pm, headOfShadith, 1)
		}
	}
}
