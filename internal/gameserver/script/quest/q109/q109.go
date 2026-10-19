// Package q109 is the quest In Search of the Nest: a Golden Ram mercenary
// takes Pierce's errand to a dead scout's body, brings its memo back to
// Pierce and reports to Kahman, for adena.
package q109

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q109_InSearchOfTheNest"

// NPCs.
const (
	pierce      = 31553
	kahman      = 31554
	scoutCorpse = 32015
)

// Items.
const (
	scoutMemo    = 8083
	recruitBadge = 7246
	soldierBadge = 7247
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "In Search of the Nest",
		QuestID: 109,
		Items:   []int32{scoutMemo},
		Bind: script.Bindings{
			script.EventQuestStart: {pierce},
			script.EventTalked:     {pierce, scoutCorpse, kahman},
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
	case strings.EqualFold(e.Name, "31553-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32015-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, scoutMemo, 1)
	case strings.EqualFold(e.Name, "31553-03.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, scoutMemo, 1)
	case strings.EqualFold(e.Name, "31554-02.htm"):
		s.RewardItems(p, 57, 5168)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		// One Golden Ram badge or the other is needed to be accepted.
		if p.Level() >= 66 && p.HasAtLeastOneItem(recruitBadge, soldierBadge) {
			htmltext = "31553-00.htm"
		} else {
			htmltext = "31553-00a.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case pierce:
			switch cond {
			case 1:
				htmltext = "31553-01a.htm"
			case 2:
				htmltext = "31553-02.htm"
			case 3:
				htmltext = "31553-03.htm"
			}
		case scoutCorpse:
			switch cond {
			case 1:
				htmltext = "32015-01.htm"
			case 2:
				htmltext = "32015-02.htm"
			}
		case kahman:
			if cond == 3 {
				htmltext = "31554-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
