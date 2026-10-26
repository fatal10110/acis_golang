// Package q650 is the quest A Broken Dream: the ghost, for a character of
// level 39 or more who has done The Ocean of Distant Stars, takes the dream
// fragments the crewmen and vagabonds drop.
package q650

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q650_ABrokenDream"

// completedQuest is the quest the ghost wants done before it talks.
const completedQuest = "Q117_TheOceanOfDistantStars"

// NPC.
const ghost = 32054

// Items.
const dreamFragment = 8514

// Monsters.
const (
	crewman  = 22027
	vagabond = 22028
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Broken Dream",
		QuestID: 650,
		Items:   []int32{dreamFragment},
		Bind: script.Bindings{
			script.EventQuestStart: {ghost},
			script.EventTalked:     {ghost},
			script.EventMyDying:    {crewman, vagabond},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, answers a visit without fragments, or ends the
// quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32054-01a.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32054-03.htm"):
		if !e.Player.HasItem(dreamFragment) {
			htmltext = "32054-04.htm"
		}
	case strings.EqualFold(e.Name, "32054-05.htm"):
		s.PlaySound(e.Player, script.SoundGiveUp)
		st.Exit(true)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		st2 := s.QuestState(e.Player, completedQuest)
		if st2 != nil && st2.IsCompleted() && e.Player.Level() >= 39 {
			htmltext = "32054-01.htm"
		} else {
			htmltext = "32054-00.htm"
			st.Exit(true)
		}
	case questlog.StatusStarted:
		htmltext = "32054-02.htm"
	}
	return htmltext
}

// onMyDying drops a dream fragment to the killer, who must have the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(p, dreamFragment, 1, 0, 250000)
}
