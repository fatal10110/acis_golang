// Package q120 is the quest Pavel's Research: after Resurrection of an Old
// Manager, the suspicious pile of stones, Wendy, Yumi, the three weather
// masters and Doctor Chaos's bookshelf walk the character through Pavel's
// research, for an earring of binding.
package q120

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q120_PavelsResearch"

// NPCs.
const (
	yumi                       = 32041
	weathermaster1             = 32042 // north
	weathermaster2             = 32043 // east
	weathermaster3             = 32044 // west
	doctorChaosSecretBookshelf = 32045
	suspiciousPileOfStones     = 32046
	wendy                      = 32047
)

// Items.
const (
	lockupResearchReport = 8058
	researchReport       = 8059
	keyOfEnigma          = 8060
	flowerOfPavel        = 8290
	heartOfAtlanta       = 8291
	wendyNecklace        = 8292
)

// earringOfBinding is the reward.
const earringOfBinding = 854

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Pavel's Research",
		QuestID: 120,
		Items:   []int32{lockupResearchReport, researchReport, keyOfEnigma, flowerOfPavel, heartOfAtlanta, wendyNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {suspiciousPileOfStones},
			script.EventTalked:     {yumi, weathermaster1, weathermaster2, weathermaster3, doctorChaosSecretBookshelf, suspiciousPileOfStones, wendy},
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
	is := func(name string) bool { return strings.EqualFold(e.Name, name) }
	switch {
	case is("32041-03.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case is("32041-04.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
	case is("32041-12.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
	case is("32041-16.htm"):
		st.SetCond(16)
		s.GiveItems(p, keyOfEnigma, 1)
		s.PlaySound(p, script.SoundMiddle)
	case is("32041-22.htm"):
		st.SetCond(17)
		s.TakeItems(p, keyOfEnigma, 1)
		s.PlaySound(p, script.SoundMiddle)
	case is("32041-32.htm"):
		s.TakeItems(p, wendyNecklace, 1)
		s.GiveItems(p, earringOfBinding, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case is("32042-06.htm"):
		if st.Cond() == 10 {
			if st.GetInt("talk")+st.GetInt("talk1") == 2 {
				st.SetCond(11)
				st.Set("talk", strconv.Itoa(0))
				st.Set("talk1", strconv.Itoa(0))
				s.PlaySound(p, script.SoundMiddle)
			} else {
				htmltext = "32042-03.htm"
			}
		}
	case is("32042-08.htm"):
		s.PlaySound(p, "AmbSound.dt_percussion_01")
	case is("32042-10.htm"):
		if st.GetInt("talk")+st.GetInt("talk1")+st.GetInt("talk2") == 3 {
			htmltext = "32042-14.htm"
		}
	case is("32042-11.htm"):
		if st.GetInt("talk") == 0 {
			st.Set("talk", strconv.Itoa(1))
		}
	case is("32042-12.htm"):
		if st.GetInt("talk1") == 0 {
			st.Set("talk1", strconv.Itoa(1))
		}
	case is("32042-13.htm"):
		if st.GetInt("talk2") == 0 {
			st.Set("talk2", strconv.Itoa(1))
		}
	case is("32042-15.htm"):
		st.SetCond(12)
		st.Set("talk", strconv.Itoa(0))
		st.Set("talk1", strconv.Itoa(0))
		st.Set("talk2", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
	case is("32043-06.htm"):
		if st.Cond() == 17 {
			if st.GetInt("talk")+st.GetInt("talk1") == 2 {
				st.SetCond(18)
				st.Set("talk", strconv.Itoa(0))
				st.Set("talk1", strconv.Itoa(0))
				s.PlaySound(p, script.SoundMiddle)
			} else {
				htmltext = "32043-03.htm"
			}
		}
	case is("32043-15.htm"):
		if st.GetInt("talk")+st.GetInt("talk1") == 2 {
			htmltext = "32043-29.htm"
		}
	case is("32043-18.htm"):
		if st.GetInt("talk") == 1 {
			htmltext = "32043-21.htm"
		}
	case is("32043-20.htm"):
		st.Set("talk", strconv.Itoa(1))
		s.PlaySound(p, "AmbSound.ed_drone_02")
	case is("32043-28.htm"):
		st.Set("talk1", strconv.Itoa(1))
	case is("32043-30.htm"):
		st.SetCond(19)
		st.Set("talk", strconv.Itoa(0))
		st.Set("talk1", strconv.Itoa(0))
	case is("32044-06.htm"):
		if st.Cond() == 20 {
			if st.GetInt("talk")+st.GetInt("talk1") == 2 {
				st.SetCond(21)
				st.Set("talk", strconv.Itoa(0))
				st.Set("talk1", strconv.Itoa(0))
				s.PlaySound(p, script.SoundMiddle)
				s.PlaySound(p, "AmbSound.ac_percussion_02")
			} else {
				htmltext = "32044-03.htm"
			}
		}
	case is("32044-08.htm"):
		if st.GetInt("talk")+st.GetInt("talk1") == 2 {
			htmltext = "32044-11.htm"
		}
	case is("32044-09.htm"):
		if st.GetInt("talk") == 0 {
			st.Set("talk", strconv.Itoa(1))
		}
	case is("32044-10.htm"):
		if st.GetInt("talk1") == 0 {
			st.Set("talk1", strconv.Itoa(1))
		}
	case is("32044-17.htm"):
		st.SetCond(22)
		st.Set("talk", strconv.Itoa(0))
		st.Set("talk1", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
		s.PlaySound(p, "AmbSound.ed_drone_02")
		e.NPC.AddCastDesire(p, skill.Ref{ID: 5073, Level: 5}, 1000000)
	case is("32045-02.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, lockupResearchReport, 1)
		e.NPC.AddCastDesire(p, skill.Ref{ID: 5073, Level: 5}, 1000000)
	case is("32046-04.htm") || is("32046-05.htm"):
		st.Exit(true)
	case is("32046-06.htm"):
		if p.Level() >= 50 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "32046-00.htm"
			st.Exit(true)
		}
	case is("32046-08.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case is("32046-12.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, flowerOfPavel, 1)
	case is("32046-22.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
	case is("32046-29.htm"):
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
	case is("32046-35.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
	case is("32046-38.htm"):
		st.SetCond(23)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, heartOfAtlanta, 1)
	case is("32047-06.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case is("32047-10.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, flowerOfPavel, 1)
	case is("32047-15.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case is("32047-18.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
	case is("32047-26.htm"):
		st.SetCond(24)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, heartOfAtlanta, 1)
	case is("32047-32.htm"):
		st.SetCond(25)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, wendyNecklace, 1)
	case is("w1_1"):
		st.Set("talk", strconv.Itoa(1))
		htmltext = "32042-04.htm"
	case is("w1_2"):
		st.Set("talk1", strconv.Itoa(1))
		htmltext = "32042-05.htm"
	case is("w2_1"):
		st.Set("talk", strconv.Itoa(1))
		htmltext = "32043-04.htm"
	case is("w2_2"):
		st.Set("talk1", strconv.Itoa(1))
		htmltext = "32043-05.htm"
	case is("w3_1"):
		st.Set("talk", strconv.Itoa(1))
		htmltext = "32044-04.htm"
	case is("w3_2"):
		st.Set("talk1", strconv.Itoa(1))
		htmltext = "32044-05.htm"
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
		st2 := s.QuestState(p, "Q114_ResurrectionOfAnOldManager")
		switch {
		case st2 != nil && st2.IsCompleted():
			if p.Level() < 50 {
				htmltext = "32046-00.htm"
			} else {
				htmltext = "32046-01.htm"
			}
		default:
			htmltext = "32046-00.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e, st, htmltext)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page an NPC of the started quest shows, htmltext when
// it has none at the quest's cond.
func talkStarted(s *script.Script, e script.Talk, st *script.QuestState, htmltext string) string {
	p := e.Player
	cond := st.Cond()
	switch e.NPC.NpcID() {
	case suspiciousPileOfStones:
		switch {
		case cond == 1:
			htmltext = "32046-06.htm"
		case cond > 1 && cond < 5:
			htmltext = "32046-09.htm"
		case cond == 5:
			htmltext = "32046-10.htm"
		case cond > 5 && cond < 9:
			htmltext = "32046-13.htm"
		case cond == 9:
			htmltext = "32046-14.htm"
		case cond == 10 || cond == 11:
			htmltext = "32046-23.htm"
		case cond == 12:
			htmltext = "32046-26.htm"
		case cond > 12 && cond < 19:
			htmltext = "32046-30.htm"
		case cond == 19:
			htmltext = "32046-31.htm"
		case cond == 20 || cond == 21:
			htmltext = "32046-36.htm"
		case cond == 22:
			htmltext = "32046-37.htm"
		case cond > 22:
			htmltext = "32046-39.htm"
		}
	case wendy:
		switch {
		case cond == 2 || cond == 3 || cond == 4:
			htmltext = "32047-01.htm"
		case cond == 5:
			htmltext = "32047-07.htm"
		case cond == 6:
			htmltext = "32047-08.htm"
		case cond == 7:
			htmltext = "32047-11.htm"
		case cond == 8:
			htmltext = "32047-12.htm"
		case cond > 8 && cond < 13:
			htmltext = "32047-15.htm"
		case cond == 13:
			htmltext = "32047-16.htm"
		case cond == 14:
			htmltext = "32047-19.htm"
		case cond > 14 && cond < 23:
			htmltext = "32047-20.htm"
		case cond == 23:
			htmltext = "32047-21.htm"
		case cond == 24:
			htmltext = "32047-26.htm"
		case cond == 25:
			htmltext = "32047-33.htm"
		}
	case yumi:
		switch {
		case cond == 2:
			htmltext = "32041-01.htm"
		case cond == 3:
			htmltext = "32041-05.htm"
		case cond > 3 && cond < 7:
			htmltext = "32041-06.htm"
		case cond == 7:
			htmltext = "32041-07.htm"
		case cond > 7 && cond < 15:
			htmltext = "32041-13.htm"
		case cond == 15:
			htmltext = "32041-14.htm"
		case cond == 16:
			if !p.HasItem(researchReport) {
				htmltext = "32041-17.htm"
			} else {
				htmltext = "32041-18.htm"
			}
		case cond > 16 && cond < 25:
			htmltext = "32041-22.htm"
		case cond == 25:
			htmltext = "32041-26.htm"
		}
	case weathermaster1:
		switch {
		case cond == 10:
			htmltext = "32042-01.htm"
			s.PlaySound(p, "AmbSound.cd_crystal_loop")
		case cond == 11:
			if st.GetInt("talk")+st.GetInt("talk1")+st.GetInt("talk2") == 3 {
				htmltext = "32042-14.htm"
			} else {
				htmltext = "32042-06.htm"
			}
		case cond > 11:
			htmltext = "32042-15.htm"
		}
	case weathermaster2:
		switch {
		case cond == 17:
			htmltext = "32043-01.htm"
		case cond == 18:
			if st.GetInt("talk")+st.GetInt("talk1") == 2 {
				htmltext = "32043-29.htm"
			} else {
				htmltext = "32043-06.htm"
			}
		case cond > 18:
			htmltext = "32043-30.htm"
		}
	case weathermaster3:
		switch {
		case cond == 20:
			htmltext = "32044-01.htm"
		case cond == 21:
			htmltext = "32044-06.htm"
		case cond > 21:
			htmltext = "32044-18.htm"
		}
	case doctorChaosSecretBookshelf:
		switch {
		case cond == 14:
			htmltext = "32045-01.htm"
		case cond > 14:
			htmltext = "32045-03.htm"
		}
	}
	return htmltext
}
