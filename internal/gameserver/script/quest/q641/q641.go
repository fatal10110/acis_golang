// Package q641 is the quest Attack Sailren!: the statue, for a character of
// level 77 or more who has done The Name of Evil, Part 2, asks for thirty
// Gazkh fragments and pays a Gazkh with them.
package q641

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q641_AttackSailren"

// completedQuest is the quest the statue wants done before it talks.
const completedQuest = "Q126_TheNameOfEvil_2"

// NPC.
const statue = 32109

// Items.
const (
	gazkhFragment = 8782
	gazkh         = 8784
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Attack Sailren!",
		QuestID: 641,
		Items:   []int32{gazkhFragment},
		Bind: script.Bindings{
			script.EventQuestStart: {statue},
			script.EventTalked:     {statue},
			script.EventMyDying:    {22196, 22197, 22198, 22199, 22218, 22223},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or takes the fragments in for the Gazkh while
// the statue shows its cast.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		// No page is shown.
		return ""
	}
	switch {
	case strings.EqualFold(e.Name, "32109-5.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32109-8.htm"):
		if e.Player.ItemCount(gazkhFragment) >= 30 {
			e.NPC.BroadcastMagicSkillUse(e.NPC, e.Player, 5089, 1, 3000, 0)
			s.TakeItems(e.Player, gazkhFragment, -1)
			s.GiveItems(e.Player, gazkh, 1)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "32109-6.htm"
			st.SetCond(1)
		}
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
		if e.Player.Level() < 77 {
			htmltext = "32109-3.htm"
		} else {
			st2 := s.QuestState(e.Player, completedQuest)
			if st2 != nil && st2.IsCompleted() {
				htmltext = "32109-1.htm"
			} else {
				htmltext = "32109-2.htm"
			}
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32109-5.htm"
		case 2:
			htmltext = "32109-7.htm"
		}
	}
	return htmltext
}

// onMyDying drops a Gazkh fragment to a party member at cond 1, and moves the
// one who then holds thirty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), gazkhFragment, 1, 30, 50000) {
		st.SetCond(2)
	}
}
