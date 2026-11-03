// Package q347 is the quest Go Get the Calculator: Brunon wants a calculator
// that Silvera builds from gemstone beast crystals.
package q347

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q347_GoGetTheCalculator"

// NPCs.
const (
	brunon  = 30526
	silvera = 30527
	spiron  = 30532
	balanki = 30533
)

// Items.
const (
	gemstoneBeastCrystal = 4286
	calculatorQuest      = 4285
	calculatorReal       = 4393
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Go Get the Calculator",
		QuestID: 347,
		Items:   []int32{gemstoneBeastCrystal, calculatorQuest},
		Bind: script.Bindings{
			script.EventQuestStart: {brunon},
			script.EventTalked:     {brunon, silvera, spiron, balanki},
			script.EventMyDying:    {20540},
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
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30526-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30533-03.htm"):
		if p.Adena() >= 100 {
			htmltext = "30533-02.htm"
			s.TakeItems(p, 57, 100)
			if st.Cond() == 3 {
				st.SetCond(4)
			} else {
				st.SetCond(2)
			}
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30532-02.htm"):
		if st.Cond() == 2 {
			st.SetCond(4)
		} else {
			st.SetCond(3)
		}
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30526-08.htm"):
		s.TakeItems(p, calculatorQuest, -1)
		s.GiveItems(p, calculatorReal, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30526-09.htm"):
		s.TakeItems(p, calculatorQuest, -1)
		s.RewardItems(p, 57, 1000)
		s.PlaySound(p, script.SoundFinish)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 12 {
			htmltext = "30526-00.htm"
		} else {
			htmltext = "30526-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case brunon:
			if !p.HasItem(calculatorQuest) {
				htmltext = "30526-06.htm"
			} else {
				htmltext = "30526-07.htm"
			}
		case spiron:
			if cond < 4 {
				htmltext = "30532-01.htm"
			} else {
				htmltext = "30532-05.htm"
			}
		case balanki:
			if cond < 4 {
				htmltext = "30533-01.htm"
			} else {
				htmltext = "30533-04.htm"
			}
		case silvera:
			switch {
			case cond < 4:
				htmltext = "30527-00.htm"
			case cond == 4:
				htmltext = "30527-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 5:
				if p.ItemCount(gemstoneBeastCrystal) < 10 {
					htmltext = "30527-02.htm"
				} else {
					htmltext = "30527-03.htm"
					st.SetCond(6)
					s.TakeItems(p, gemstoneBeastCrystal, -1)
					s.GiveItems(p, calculatorQuest, 1)
					s.PlaySound(p, script.SoundMiddle)
				}
			case cond == 6:
				htmltext = "30527-04.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 5) == nil {
		return
	}
	s.DropItems(p, gemstoneBeastCrystal, 1, 10, 500000)
}
