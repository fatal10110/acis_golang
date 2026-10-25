// Package q127 is the quest Kamael: A Window to the Future: Dominic sends
// the character to the masters of each race for their marks, and Rodemai
// plays the Kamael's story scene.
package q127

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q127_KamaelAWindowToTheFuture"

// NPCs.
const (
	dominic = 31350
	klaus   = 30187
	alder   = 32092
	aklan   = 31288
	oltlin  = 30862
	juris   = 30113
	rodemai = 30756
)

// Items.
const (
	markDominic = 8939
	markHuman   = 8940
	markDwarf   = 8941
	markOrc     = 8944
	markDelf    = 8943
	markElf     = 8942
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Kamael: A Window to the Future",
		QuestID: 127,
		Items:   []int32{markDominic, markHuman, markDwarf, markOrc, markDelf, markElf},
		Bind: script.Bindings{
			script.EventQuestStart: {dominic},
			script.EventTalked:     {dominic, klaus, alder, aklan, oltlin, juris, rodemai},
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
	case strings.EqualFold(e.Name, "31350-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, markDominic, 1)
	case strings.EqualFold(e.Name, "31350-06.htm"):
		s.TakeItems(p, markHuman, -1)
		s.TakeItems(p, markDwarf, -1)
		s.TakeItems(p, markElf, -1)
		s.TakeItems(p, markDelf, -1)
		s.TakeItems(p, markOrc, -1)
		s.TakeItems(p, markDominic, -1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "30187-06.htm"):
		st.SetCond(2)
	case strings.EqualFold(e.Name, "30187-08.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, markHuman, 1)
	case strings.EqualFold(e.Name, "32092-05.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, markDwarf, 1)
	case strings.EqualFold(e.Name, "31288-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, markOrc, 1)
	case strings.EqualFold(e.Name, "30862-04.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, markDelf, 1)
	case strings.EqualFold(e.Name, "30113-04.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, markElf, 1)
	case strings.EqualFold(e.Name, "kamaelstory"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		p.PlayScene()
		return ""
	case strings.EqualFold(e.Name, "30756-05.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	cond := st.Cond()
	switch st.Status() {
	case questlog.StatusCreated:
		htmltext = "31350-01.htm"
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case klaus:
			if cond == 1 {
				htmltext = "30187-01.htm"
			} else if cond == 2 {
				htmltext = "30187-06.htm"
			}
		case alder:
			if cond == 3 {
				htmltext = "32092-01.htm"
			}
		case aklan:
			if cond == 4 {
				htmltext = "31288-01.htm"
			}
		case oltlin:
			if cond == 5 {
				htmltext = "30862-01.htm"
			}
		case juris:
			if cond == 6 {
				htmltext = "30113-01.htm"
			}
		case rodemai:
			if cond == 7 {
				htmltext = "30756-01.htm"
			} else if cond == 8 {
				htmltext = "30756-04.htm"
			}
		case dominic:
			if cond == 9 {
				htmltext = "31350-05.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
