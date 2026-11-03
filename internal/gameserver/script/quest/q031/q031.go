// Package q031 is the quest Secret Buried in the Swamp: Abercrombie sends the
// player to a dwarf's corpse for Krorin's journal, then around the four
// forgotten monuments, and pays adena and experience for the journal.
package q031

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q031_SecretBuriedInTheSwamp"

// krorinJournal is the quest item.
const krorinJournal = 7252

// NPCs.
const (
	abercrombie        = 31555
	forgottenMonument1 = 31661
	forgottenMonument2 = 31662
	forgottenMonument3 = 31663
	forgottenMonument4 = 31664
	corpseOfDwarf      = 31665
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Secret Buried in the Swamp",
		QuestID: 31,
		Items:   []int32{krorinJournal},
		Bind: script.Bindings{
			script.EventQuestStart: {abercrombie},
			script.EventTalked: {
				abercrombie, corpseOfDwarf,
				forgottenMonument1, forgottenMonument2, forgottenMonument3, forgottenMonument4,
			},
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
	case strings.EqualFold(e.Name, "31555-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31665-01.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, krorinJournal, 1)
	case strings.EqualFold(e.Name, "31555-04.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31661-01.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31662-01.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31663-01.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31664-01.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31555-07.htm"):
		s.TakeItems(p, krorinJournal, 1)
		s.RewardItems(p, 57, 40000)
		s.RewardExpAndSp(p, 130000, 0)
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
		if e.Player.Level() < 66 {
			htmltext = "31555-00a.htm"
		} else {
			htmltext = "31555-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case abercrombie:
			switch {
			case cond == 1:
				htmltext = "31555-02.htm"
			case cond == 2:
				htmltext = "31555-03.htm"
			case cond > 2 && cond < 7:
				htmltext = "31555-05.htm"
			case cond == 7:
				htmltext = "31555-06.htm"
			}
		case corpseOfDwarf:
			if cond == 1 {
				htmltext = "31665-00.htm"
			} else if cond > 1 {
				htmltext = "31665-02.htm"
			}
		case forgottenMonument1:
			if cond == 3 {
				htmltext = "31661-00.htm"
			} else if cond > 3 {
				htmltext = "31661-02.htm"
			}
		case forgottenMonument2:
			if cond == 4 {
				htmltext = "31662-00.htm"
			} else if cond > 4 {
				htmltext = "31662-02.htm"
			}
		case forgottenMonument3:
			if cond == 5 {
				htmltext = "31663-00.htm"
			} else if cond > 5 {
				htmltext = "31663-02.htm"
			}
		case forgottenMonument4:
			if cond == 6 {
				htmltext = "31664-00.htm"
			} else if cond > 6 {
				htmltext = "31664-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
