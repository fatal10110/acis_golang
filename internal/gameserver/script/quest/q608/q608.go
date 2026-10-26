// Package q608 is the quest Slay the Enemy Commander: Kadun Zu Ketra asks a
// Ketra ally of level 4 or more for the head of Mos, the Varka raid boss,
// and pays for it with the Totem of Wisdom. Every ally of the killer's
// party near Mos gets a head.
package q608

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q608_SlayTheEnemyCommander"

// kadun is the quest giver, Kadun Zu Ketra.
const kadun = 31370

// mos is the raid boss whose head the quest asks for.
const mos = 25312

// Items.
const (
	headOfMos      = 7236
	totemOfWisdom  = 7220
	ketraAlliance4 = 7214
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Slay the enemy commander!",
		QuestID: 608,
		Items:   []int32{headOfMos},
		Bind: script.Bindings{
			script.EventQuestStart: {kadun},
			script.EventTalked:     {kadun},
			script.EventMyDying:    {mos},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the head for the totem. Without
// the head the quest goes back to cond 1.
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
		if e.Player.HasItem(headOfMos) {
			s.TakeItems(e.Player, headOfMos, -1)
			s.GiveItems(e.Player, totemOfWisdom, 1)
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

// onTalk offers the quest to a level 75 Ketra ally of level 4 or more who
// holds the level 4 alliance mark and no totem yet.
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
		case e.Player.VarkaKetraAlliance() >= 4 && e.Player.HasItem(ketraAlliance4) && !e.Player.HasItem(totemOfWisdom):
			htmltext = "31370-01.htm"
		default:
			htmltext = "31370-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(headOfMos) {
			htmltext = "31370-05.htm"
		} else {
			htmltext = "31370-06.htm"
		}
	}
	return htmltext
}

// onMyDying gives a head to every party member at cond 1 near Mos who is a
// Ketra ally of level 4 or more and holds the alliance mark, and moves each
// to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(1)) {
		pm := st.Player()
		if pm.VarkaKetraAlliance() >= 4 && pm.HasItem(ketraAlliance4) {
			st.SetCond(2)
			s.PlaySound(pm, script.SoundMiddle)
			s.GiveItems(pm, headOfMos, 1)
		}
	}
}
