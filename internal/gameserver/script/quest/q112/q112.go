// Package q112 is the quest Walk of Fate: Livina sends the character to
// Karuda, who pays with an enchant scroll and adena.
package q112

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q112_WalkOfFate"

// NPCs.
const (
	livina = 30572
	karuda = 32017
)

// enchantD is the reward scroll.
const enchantD = 956

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Walk of Fate",
		QuestID: 112,
		Bind: script.Bindings{
			script.EventQuestStart: {livina},
			script.EventTalked:     {livina, karuda},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30572-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32017-02.htm"):
		s.GiveItems(p, enchantD, 1)
		s.RewardItems(p, 57, 4665)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		if e.Player.Level() < 20 {
			htmltext = "30572-00.htm"
		} else {
			htmltext = "30572-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case livina:
			htmltext = "30572-03.htm"
		case karuda:
			htmltext = "32017-01.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
