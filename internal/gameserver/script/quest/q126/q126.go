// Package q126 is the quest The Name of Evil - 2: Asamah sends the
// character to the three Kaimu stones and the warrior's grave, whose three
// songs it plays, then to the Shilen stone statue with the bone powder, and
// Mushika rewards it with an enchant scroll.
package q126

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q126_TheNameOfEvil_2"

// NPCs.
const (
	mushika           = 32114
	asamanah          = 32115
	uluKaimu          = 32119
	baluKaimu         = 32120
	chutaKaimu        = 32121
	warriorGrave      = 32122
	shilenStoneStatue = 32109
)

// Items.
const (
	bonePowder = 8783
	ewa        = 729
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Name of Evil - 2",
		QuestID: 126,
		Bind: script.Bindings{
			script.EventQuestStart: {asamanah},
			script.EventTalked:     {asamanah, mushika, uluKaimu, baluKaimu, chutaKaimu, warriorGrave, shilenStoneStatue},
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
	case strings.EqualFold(e.Name, "32115-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32115-10.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32119-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32119-09.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32119-11.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32120-07.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32120-09.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32120-11.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32121-07.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32121-10.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32121-15.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32122-03.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32122-15.htm"):
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32122-18.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32122-87.htm"):
		s.GiveItems(p, bonePowder, 1)
	case strings.EqualFold(e.Name, "32122-90.htm"):
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32109-02.htm"):
		st.SetCond(19)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32109-19.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bonePowder, 1)
	case strings.EqualFold(e.Name, "32115-21.htm"):
		st.SetCond(21)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32115-28.htm"):
		st.SetCond(22)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32114-08.htm"):
		st.SetCond(23)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32114-09.htm"):
		s.GiveItems(p, ewa, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "DOOne"):
		htmltext = "32122-26.htm"
		if st.GetInt("DO") < 1 {
			st.Set("DO", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "MIOne"):
		htmltext = "32122-30.htm"
		if st.GetInt("MI") < 1 {
			st.Set("MI", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "FAOne"):
		htmltext = "32122-34.htm"
		if st.GetInt("FA") < 1 {
			st.Set("FA", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "SOLOne"):
		htmltext = "32122-38.htm"
		if st.GetInt("SOL") < 1 {
			st.Set("SOL", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "FA_2One"):
		if st.GetInt("FA_2") < 1 {
			st.Set("FA_2", strconv.Itoa(1))
		}
		htmltext = getSongOne(s, p, st)
	case strings.EqualFold(e.Name, "FATwo"):
		htmltext = "32122-47.htm"
		if st.GetInt("FA") < 1 {
			st.Set("FA", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "SOLTwo"):
		htmltext = "32122-51.htm"
		if st.GetInt("SOL") < 1 {
			st.Set("SOL", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "TITwo"):
		htmltext = "32122-55.htm"
		if st.GetInt("TI") < 1 {
			st.Set("TI", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "SOL_2Two"):
		htmltext = "32122-59.htm"
		if st.GetInt("SOL_2") < 1 {
			st.Set("SOL_2", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "FA_2Two"):
		if st.GetInt("FA_2") < 1 {
			st.Set("FA_2", strconv.Itoa(1))
		}
		htmltext = getSongTwo(s, p, st)
	case strings.EqualFold(e.Name, "SOLTri"):
		htmltext = "32122-68.htm"
		if st.GetInt("SOL") < 1 {
			st.Set("SOL", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "FATri"):
		htmltext = "32122-72.htm"
		if st.GetInt("FA") < 1 {
			st.Set("FA", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "MITri"):
		htmltext = "32122-76.htm"
		if st.GetInt("MI") < 1 {
			st.Set("MI", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "FA_2Tri"):
		htmltext = "32122-80.htm"
		if st.GetInt("FA_2") < 1 {
			st.Set("FA_2", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "MI_2Tri"):
		if st.GetInt("MI_2") < 1 {
			st.Set("MI_2", strconv.Itoa(1))
		}
		htmltext = getSongTri(s, p, st)
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
		if p.Level() < 77 {
			htmltext = "32115-02.htm"
		} else {
			st2 := s.QuestState(p, "Q125_TheNameOfEvil_1")
			if st2 != nil && st2.IsCompleted() {
				htmltext = "32115-01.htm"
			} else {
				htmltext = "32115-04.htm"
			}
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case asamanah:
			switch {
			case cond == 1:
				htmltext = "32115-11.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			case cond > 1 && cond < 20:
				htmltext = "32115-12.htm"
			case cond == 20:
				htmltext = "32115-13.htm"
			case cond == 21:
				htmltext = "32115-22.htm"
			case cond == 22:
				htmltext = "32115-29.htm"
			}
		case uluKaimu:
			switch {
			case cond == 1:
				htmltext = "32119-01a.htm"
			case cond == 2:
				htmltext = "32119-02.htm"
			case cond == 3:
				htmltext = "32119-08.htm"
			case cond == 4:
				htmltext = "32119-09.htm"
			case cond > 4:
				htmltext = "32119-12.htm"
			}
		case baluKaimu:
			switch {
			case cond < 5:
				htmltext = "32120-02.htm"
			case cond == 5:
				htmltext = "32120-01.htm"
			case cond == 6:
				htmltext = "32120-03.htm"
			case cond == 7:
				htmltext = "32120-08.htm"
			case cond > 7:
				htmltext = "32120-12.htm"
			}
		case chutaKaimu:
			switch {
			case cond < 8:
				htmltext = "32121-02.htm"
			case cond == 8:
				htmltext = "32121-01.htm"
			case cond == 9:
				htmltext = "32121-03.htm"
			case cond == 10:
				htmltext = "32121-10.htm"
			case cond > 10:
				htmltext = "32121-16.htm"
			}
		case warriorGrave:
			switch {
			case cond < 11:
				htmltext = "32122-02.htm"
			case cond == 11:
				htmltext = "32122-01.htm"
			case cond == 12:
				htmltext = "32122-15.htm"
			case cond == 13:
				htmltext = "32122-18.htm"
			case cond == 14:
				htmltext = "32122-24.htm"
			case cond == 15:
				htmltext = "32122-45.htm"
			case cond == 16:
				htmltext = "32122-66.htm"
			case cond == 17:
				htmltext = "32122-84.htm"
			case cond == 18:
				htmltext = "32122-91.htm"
			}
		case shilenStoneStatue:
			switch {
			case cond < 18:
				htmltext = "32109-03.htm"
			case cond == 18:
				htmltext = "32109-02.htm"
			case cond == 19:
				htmltext = "32109-05.htm"
			case cond > 19:
				htmltext = "32109-04.htm"
			}
		case mushika:
			switch {
			case cond < 22:
				htmltext = "32114-02.htm"
			case cond == 22:
				htmltext = "32114-01.htm"
			case cond == 23:
				htmltext = "32114-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// getSongOne ends the first song once its five notes are played.
func getSongOne(s *script.Script, p *script.Player, st *script.QuestState) string {
	htmltext := "32122-24.htm"
	if st.Cond() == 14 && st.GetInt("DO") > 0 && st.GetInt("MI") > 0 && st.GetInt("FA") > 0 && st.GetInt("SOL") > 0 && st.GetInt("FA_2") > 0 {
		htmltext = "32122-42.htm"
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		st.Unset("DO")
		st.Unset("MI")
		st.Unset("FA")
		st.Unset("SOL")
		st.Unset("FA_2")
	}
	return htmltext
}

// getSongTwo ends the second song once its five notes are played. It
// clears FA3_2, a variable nothing sets, so FA_2 stays set into the third
// song.
func getSongTwo(s *script.Script, p *script.Player, st *script.QuestState) string {
	htmltext := "32122-45.htm"
	if st.Cond() == 15 && st.GetInt("FA") > 0 && st.GetInt("SOL") > 0 && st.GetInt("TI") > 0 && st.GetInt("SOL_2") > 0 && st.GetInt("FA_2") > 0 {
		htmltext = "32122-63.htm"
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
		st.Unset("FA")
		st.Unset("SOL")
		st.Unset("TI")
		st.Unset("SOL_2")
		st.Unset("FA3_2")
	}
	return htmltext
}

// getSongTri ends the third song once its five notes are played.
func getSongTri(s *script.Script, p *script.Player, st *script.QuestState) string {
	htmltext := "32122-66.htm"
	if st.Cond() == 16 && st.GetInt("SOL") > 0 && st.GetInt("FA") > 0 && st.GetInt("MI") > 0 && st.GetInt("FA_2") > 0 && st.GetInt("MI_2") > 0 {
		htmltext = "32122-84.htm"
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
		st.Unset("SOL")
		st.Unset("FA")
		st.Unset("MI")
		st.Unset("FA_2")
		st.Unset("MI_2")
	}
	return htmltext
}
