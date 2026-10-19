// Package q163 is the quest Legacy of the Poet: Starden pays adena for the
// four pages of Rumiel's poems the Ol Mahum carry.
package q163

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q163_LegacyOfThePoet"

// NPC.
const starden = 30220

// rumielsPoems returns the four poem pages.
func rumielsPoems() []int32 {
	return []int32{1038, 1039, 1040, 1041}
}

// droplist returns the drop of each poem page.
func droplist() []script.DropInfo {
	poems := rumielsPoems()
	return []script.DropInfo{
		{ItemID: poems[0], Count: 1, Needed: 1, Chance: 100000},
		{ItemID: poems[1], Count: 1, Needed: 1, Chance: 200000},
		{ItemID: poems[2], Count: 1, Needed: 1, Chance: 200000},
		{ItemID: poems[3], Count: 1, Needed: 1, Chance: 400000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Legacy of the Poet",
		QuestID: 163,
		Items:   rumielsPoems(),
		Bind: script.Bindings{
			script.EventQuestStart: {starden},
			script.EventTalked:     {starden},
			script.EventMyDying:    {20372, 20373},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30220-07.htm") {
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
		switch {
		case p.Race() == player.RaceDarkElf:
			htmltext = "30220-00.htm"
		case p.Level() < 11:
			htmltext = "30220-02.htm"
		default:
			htmltext = "30220-03.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 2 {
			htmltext = "30220-09.htm"
			for _, poem := range rumielsPoems() {
				s.TakeItems(p, poem, -1)
			}
			s.RewardItems(p, 57, 13890)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30220-08.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropMultipleItems(p, droplist(), script.DropDivmod) {
		st.SetCond(2)
	}
}
