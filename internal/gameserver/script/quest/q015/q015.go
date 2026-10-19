// Package q015 is the quest Sweet Whispers: Vladimir sends a player to the
// mysterious necromancer, then to the hierarch, for experience.
package q015

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q015_SweetWhispers"

// NPCs.
const (
	vladimir        = 31302
	hierarch        = 31517
	mysteriousNecro = 31518
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sweet Whispers",
		QuestID: 15,
		Bind: script.Bindings{
			script.EventQuestStart: {vladimir},
			script.EventTalked:     {vladimir, hierarch, mysteriousNecro},
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
	switch {
	case strings.EqualFold(e.Name, "31302-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31518-01.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31517-01.htm"):
		s.RewardExpAndSp(e.Player, 60217, 0)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(false)
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
			htmltext = "31302-00a.htm"
		} else {
			htmltext = "31302-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case vladimir:
			htmltext = "31302-01a.htm"
		case mysteriousNecro:
			switch cond {
			case 1:
				htmltext = "31518-00.htm"
			case 2:
				htmltext = "31518-01a.htm"
			}
		case hierarch:
			if cond == 2 {
				htmltext = "31517-00.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
