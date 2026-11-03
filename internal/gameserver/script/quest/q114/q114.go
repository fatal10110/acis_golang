// Package q114 is the quest Resurrection of an Old Manager: after Pavel the
// Giant, Yumi and Wendy walk the character through the story of the old
// manager of Hot Springs, a golem Wendy wakes, the box of the starstone, the
// detector at the suspicious pile of stones and Newyear's letter.
package q114

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q114_ResurrectionOfAnOldManager"

// NPCs.
const (
	newyear = 31961
	yumi    = 32041
	stone   = 32046
	wendy   = 32047
	box     = 32050
)

// Items.
const (
	letter     = 8288
	detector   = 8090
	detector2  = 8091
	starstone  = 8287
	starstone2 = 8289
)

// golem is the golem Wendy wakes.
const golem = 27318

// NpcString lines.
const (
	sayGolemWakes    = 11450
	sayGolemLeaves   = 11451
	sayGolemDies     = 11452
	screenStoneFound = 11453
)

// golemSpawn is where Wendy wakes the golem.
func golemSpawn() script.Loc { return script.Loc{X: 96977, Y: -110625, Z: -3322, Heading: 0} }

// quest holds the golem Wendy woke, shared by every player: one golem at a
// time, until it decays.
type quest struct{ golem *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{golem: new(script.NPCCell)}
	return script.Script{
		Title:   "Resurrection of an Old Manager",
		QuestID: 114,
		Items:   []int32{letter, detector, detector2, starstone, starstone2},
		Bind: script.Bindings{
			script.EventQuestStart: {yumi},
			script.EventTalked:     {yumi, wendy, box, stone, newyear},
			script.EventDecayed:    {golem},
			script.EventMyDying:    {golem},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: onTalk, OnDecayed: q.onDecayed, OnMyDying: onMyDying},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "32041-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("talk", strconv.Itoa(0))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32041-06.htm"):
		st.Set("talk", strconv.Itoa(1))
	case strings.EqualFold(e.Name, "32041-07.htm"):
		st.SetCond(2)
		st.Set("talk", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32041-10.htm"):
		switch st.GetInt("choice") {
		case 1:
			htmltext = "32041-10.htm"
		case 2:
			htmltext = "32041-10a.htm"
		case 3:
			htmltext = "32041-10b.htm"
		}
	case strings.EqualFold(e.Name, "32041-11.htm"):
		st.Set("talk", strconv.Itoa(1))
	case strings.EqualFold(e.Name, "32041-18.htm"):
		st.Set("talk", strconv.Itoa(2))
	case strings.EqualFold(e.Name, "32041-20.htm"):
		st.SetCond(6)
		st.Set("talk", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32041-25.htm"):
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, detector, 1)
	case strings.EqualFold(e.Name, "32041-28.htm"):
		st.Set("talk", strconv.Itoa(1))
		s.TakeItems(p, detector2, 1)
	case strings.EqualFold(e.Name, "32041-31.htm"):
		if st.GetInt("choice") > 1 {
			htmltext = "32041-37.htm"
		}
	case strings.EqualFold(e.Name, "32041-32.htm"):
		st.SetCond(21)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, letter, 1)
	case strings.EqualFold(e.Name, "32041-36.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32046-02.htm"):
		st.SetCond(19)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32046-06.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "32047-01.htm"):
		talk := st.GetInt("talk")
		talk1 := st.GetInt("talk1")
		switch {
		case talk == 1 && talk1 == 1:
			htmltext = "32047-04.htm"
		case talk == 2 && talk1 == 2 && st.GetInt("talk2") == 2:
			htmltext = "32047-08.htm"
		}
	case strings.EqualFold(e.Name, "32047-02.htm"):
		if st.GetInt("talk") == 0 {
			st.Set("talk", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "32047-03.htm"):
		if st.GetInt("talk1") == 0 {
			st.Set("talk1", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "32047-05.htm"):
		st.SetCond(3)
		st.Set("talk", strconv.Itoa(0))
		st.Set("choice", strconv.Itoa(1))
		st.Unset("talk1")
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-06.htm"):
		st.SetCond(4)
		st.Set("talk", strconv.Itoa(0))
		st.Set("choice", strconv.Itoa(2))
		st.Unset("talk1")
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-07.htm"):
		st.SetCond(5)
		st.Set("talk", strconv.Itoa(0))
		st.Set("choice", strconv.Itoa(3))
		st.Unset("talk1")
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-13.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-13a.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-15.htm"):
		if st.GetInt("talk") == 0 {
			st.Set("talk", strconv.Itoa(1))
		}
	case strings.EqualFold(e.Name, "32047-15a.htm"):
		if q.golem.Empty() {
			g := s.Spawner().AddSpawn(golem, golemSpawn(), true, 300000*time.Millisecond)
			if g != nil {
				q.golem.Set(g)
				g.Say(script.NpcString(sayGolemWakes, p.Name()))
				g.AddAttackDesire(p, 2000)
			}
		} else {
			htmltext = "32047-19a.htm"
		}
	case strings.EqualFold(e.Name, "32047-17a.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-20.htm"):
		st.Set("talk", strconv.Itoa(2))
	case strings.EqualFold(e.Name, "32047-23.htm"):
		st.SetCond(13)
		st.Set("talk", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-25.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, starstone, 1)
	case strings.EqualFold(e.Name, "32047-30.htm"):
		st.Set("talk", strconv.Itoa(2))
	case strings.EqualFold(e.Name, "32047-33.htm"):
		switch st.Cond() {
		case 7:
			st.SetCond(8)
			st.Set("talk", strconv.Itoa(0))
			s.PlaySound(p, script.SoundMiddle)
		case 8:
			st.SetCond(9)
			htmltext = "32047-34.htm"
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "32047-34.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32047-38.htm"):
		if p.Adena() >= 3000 {
			st.SetCond(26)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, 57, 3000)
			s.GiveItems(p, starstone2, 1)
		} else {
			htmltext = "32047-39.htm"
		}
	case strings.EqualFold(e.Name, "32050-02.htm"):
		st.Set("talk", strconv.Itoa(1))
		s.PlaySound(p, "ItemSound.armor_wood_3")
	case strings.EqualFold(e.Name, "32050-04.htm"):
		st.SetCond(14)
		st.Set("talk", strconv.Itoa(0))
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, starstone, 1)
	case strings.EqualFold(e.Name, "31961-02.htm"):
		st.SetCond(22)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letter, 1)
		s.GiveItems(p, starstone2, 1)
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
		pavelReq := s.QuestState(p, "Q121_PavelTheGiant")
		if pavelReq == nil || !pavelReq.IsCompleted() || p.Level() < 49 {
			htmltext = "32041-00.htm"
		} else {
			htmltext = "32041-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		talk := st.GetInt("talk")
		switch e.NPC.NpcID() {
		case yumi:
			htmltext = talkYumi(s, st, p, cond, talk, htmltext)
		case wendy:
			htmltext = talkWendy(s, st, p, cond, talk, htmltext)
		case box:
			switch {
			case cond == 13:
				if talk == 0 {
					htmltext = "32050-01.htm"
				} else {
					htmltext = "32050-03.htm"
				}
			case cond == 14:
				htmltext = "32050-05.htm"
			}
		case stone:
			switch {
			case st.Cond() == 17:
				st.SetCond(18)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, detector, 1)
				s.GiveItems(p, detector2, 1)
				p.ShowScreenMessage(4500, script.NpcString(screenStoneFound))
				return ""
			case cond == 18:
				htmltext = "32046-01.htm"
			case cond == 19:
				htmltext = "32046-02.htm"
			case cond == 27:
				htmltext = "32046-03.htm"
			}
		case newyear:
			switch {
			case cond == 21:
				htmltext = "31961-01.htm"
			case cond == 22:
				htmltext = "31961-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkYumi answers a talk to Yumi of a started quest at cond, keeping
// htmltext when no step matches.
func talkYumi(s *script.Script, st *script.QuestState, p *script.Player, cond, talk int32, htmltext string) string {
	switch {
	case cond == 1:
		if talk == 0 {
			htmltext = "32041-02.htm"
		} else {
			htmltext = "32041-06.htm"
		}
	case cond == 2:
		htmltext = "32041-08.htm"
	case cond > 2 && cond < 6:
		switch talk {
		case 0:
			htmltext = "32041-09.htm"
		case 1:
			htmltext = "32041-11.htm"
		default:
			htmltext = "32041-18.htm"
		}
	case cond == 6:
		htmltext = "32041-21.htm"
	case cond == 9 || cond == 12 || cond == 16:
		htmltext = "32041-22.htm"
	case cond == 17:
		htmltext = "32041-26.htm"
	case cond == 19:
		if talk == 0 {
			htmltext = "32041-27.htm"
		} else {
			htmltext = "32041-28.htm"
		}
	case cond == 20:
		htmltext = "32041-36.htm"
	case cond == 21:
		htmltext = "32041-33.htm"
	case cond == 22 || cond == 26:
		htmltext = "32041-34.htm"
		st.SetCond(27)
		s.PlaySound(p, script.SoundMiddle)
	case cond == 27:
		htmltext = "32041-35.htm"
	}
	return htmltext
}

// talkWendy answers a talk to Wendy of a started quest at cond, keeping
// htmltext when no step matches.
func talkWendy(s *script.Script, st *script.QuestState, p *script.Player, cond, talk int32, htmltext string) string {
	switch cond {
	case 2:
		switch {
		case talk == 0 && st.GetInt("talk1") == 0:
			htmltext = "32047-01.htm"
		case talk == 1 && st.GetInt("talk1") == 1:
			htmltext = "32047-04.htm"
		}
	case 3:
		htmltext = "32047-09.htm"
	case 4, 5:
		htmltext = "32047-09a.htm"
	case 6:
		switch st.GetInt("choice") {
		case 1:
			switch talk {
			case 0:
				htmltext = "32047-10.htm"
			case 1:
				htmltext = "32047-20.htm"
			}
		case 2:
			htmltext = "32047-10a.htm"
		case 3:
			switch talk {
			case 0:
				htmltext = "32047-14.htm"
			case 1:
				htmltext = "32047-15.htm"
			default:
				htmltext = "32047-20.htm"
			}
		}
	case 7:
		switch talk {
		case 0:
			htmltext = "32047-14.htm"
		case 1:
			htmltext = "32047-15.htm"
		default:
			htmltext = "32047-20.htm"
		}
	case 8:
		htmltext = "32047-30.htm"
	case 9:
		htmltext = "32047-27.htm"
	case 10:
		htmltext = "32047-14a.htm"
	case 11:
		htmltext = "32047-16a.htm"
	case 12:
		htmltext = "32047-18a.htm"
	case 13:
		htmltext = "32047-23.htm"
	case 14:
		htmltext = "32047-24.htm"
	case 15:
		htmltext = "32047-26.htm"
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
	case 16:
		htmltext = "32047-27.htm"
	case 20:
		htmltext = "32047-35.htm"
	case 26:
		htmltext = "32047-40.htm"
	}
	return htmltext
}

// onDecayed forgets the woken golem once it decays. A golem that leaves
// alive would say so, but an NPC is dead by the time its decayed hook runs,
// so the line is never said.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	if !q.golem.ClearIf(e.NPC) {
		return
	}
	if !e.NPC.Dead() {
		e.NPC.Say(script.NpcString(sayGolemLeaves, "Wendy"))
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 10)
	if st == nil {
		return
	}
	st.SetCond(11)
	s.PlaySound(p, script.SoundMiddle)
	e.NPC.Say(script.NpcString(sayGolemDies))
}
