// Package q119 is the quest Last Imperial Prince: the Nameless Spirit sends
// a character holding the antique brooch to Devorin and pays adena on its
// return.
package q119

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q119_LastImperialPrince"

// NPCs.
const (
	namelessSpirit = 31453
	devorin        = 32009
)

// antiqueBrooch is the brooch the quest asks for.
const antiqueBrooch = 7262

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Last Imperial Prince",
		QuestID: 119,
		Items:   []int32{antiqueBrooch},
		Bind: script.Bindings{
			script.EventQuestStart: {namelessSpirit},
			script.EventTalked:     {namelessSpirit, devorin},
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
	case strings.EqualFold(e.Name, "31453-04.htm"):
		if p.HasItem(antiqueBrooch) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "31453-04b.htm"
			st.Exit(true)
		}
	case strings.EqualFold(e.Name, "32009-02.htm"):
		if !p.HasItem(antiqueBrooch) {
			htmltext = "31453-02a.htm"
			st.Exit(true)
		}
	case strings.EqualFold(e.Name, "32009-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31453-07.htm"):
		s.RewardItems(p, 57, 68787)
		s.PlaySound(p, script.SoundFinish)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if !p.HasItem(antiqueBrooch) || p.Level() < 74 {
			htmltext = "31453-00a.htm"
		} else {
			htmltext = "31453-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case namelessSpirit:
			switch cond {
			case 1:
				htmltext = "31453-04a.htm"
			case 2:
				htmltext = "31453-05.htm"
			}
		case devorin:
			switch cond {
			case 1:
				htmltext = "32009-01.htm"
			case 2:
				htmltext = "32009-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = "31453-00b.htm"
	}
	return htmltext
}
