// Package q613 is the quest Prove your courage!: Ashas Varka Durai asks a
// Varka ally of level 75 or more for the head of Hekaton, the Ketra raid
// boss, and pays for it with the Feather of Valor. Every ally of the
// killer's party near Hekaton gets a head.
package q613

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q613_ProveYourCourage"

// ashas is the quest giver, Ashas Varka Durai.
const ashas = 31377

// hekaton is the raid boss whose head the quest asks for.
const hekaton = 25299

// Items.
const (
	headOfHekaton  = 7240
	featherOfValor = 7229
	varkaAlliance3 = 7223
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Prove your courage!",
		QuestID: 613,
		Items:   []int32{headOfHekaton},
		Bind: script.Bindings{
			script.EventQuestStart: {ashas},
			script.EventTalked:     {ashas},
			script.EventMyDying:    {hekaton},
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
		if e.Player.HasItem(headOfHekaton) {
			s.TakeItems(e.Player, headOfHekaton, -1)
			s.GiveItems(e.Player, featherOfValor, 1)
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

// onTalk offers the quest to a level 75 Varka ally of level 3 or more who
// holds the level 3 alliance mark and no feather yet.
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
		case e.Player.VarkaKetraAlliance() <= -3 && e.Player.HasItem(varkaAlliance3) && !e.Player.HasItem(featherOfValor):
			htmltext = "31377-01.htm"
		default:
			htmltext = "31377-02.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasItem(headOfHekaton) {
			htmltext = "31377-05.htm"
		} else {
			htmltext = "31377-06.htm"
		}
	}
	return htmltext
}

// onMyDying gives a head to every party member at cond 1 near Hekaton who is
// a Varka ally of level 3 or more and holds the alliance mark, and moves
// each to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	for _, st := range s.PartyMembers(p, e.NPC, script.ByCond(1)) {
		pm := st.Player()
		if pm.VarkaKetraAlliance() <= -3 && pm.HasItem(varkaAlliance3) {
			st.SetCond(2)
			s.PlaySound(pm, script.SoundMiddle)
			s.GiveItems(pm, headOfHekaton, 1)
		}
	}
}
