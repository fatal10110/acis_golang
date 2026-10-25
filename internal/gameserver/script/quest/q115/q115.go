// Package q115 is the quest The Other Side of Truth: Rafforty sends the
// character to Misa and back, then through the four ice sculptures in a set
// order, then to Kierre, while a suspicious man watches; the pieces of the
// tablet end it.
package q115

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q115_TheOtherSideOfTruth"

// Items.
const (
	misaLetter     = 8079
	raffortyLetter = 8080
	pieceOfTablet  = 8081
	reportPiece    = 8082
)

// NPCs.
const (
	rafforty      = 32020
	misa          = 32018
	kierre        = 32022
	sculpture1    = 32021
	sculpture2    = 32077
	sculpture3    = 32078
	sculpture4    = 32079
	suspiciousMan = 32019
)

// NpcString lines of the suspicious man.
const (
	sayManAppears   = 11550
	sayManLeaves    = 11551
	sayManReappears = 11552
	sayManLeaves2   = 11553
)

// npcValues tests the progression through the sculptures: the value to
// add, the modulo, the tested modulo value, then the tested values 1 to 4.
// ok is false for an NPC that is no sculpture.
func npcValues(npcID int32) (infos [7]int32, ok bool) {
	switch npcID {
	case 32021:
		return [7]int32{1, 2, 1, 6, 10, 12, 14}, true
	case 32077:
		return [7]int32{2, 4, 1, 5, 9, 12, 13}, true
	case 32078:
		return [7]int32{4, 8, 3, 3, 9, 10, 11}, true
	case 32079:
		return [7]int32{8, 0, 7, 3, 5, 6, 7}, true
	}
	return infos, false
}

// sculptureValues returns the progression values of the sculpture n; ok
// is false when n is no sculpture, which ends the event with no answer.
func sculptureValues(n *script.NPC) (infos [7]int32, ok bool) {
	if n == nil {
		return infos, false
	}
	return npcValues(n.NpcID())
}

// moduloOf is ex taken modulo the sculpture's modulo, ex itself for a
// modulo of 0.
func moduloOf(infos [7]int32, ex int32) int32 {
	if infos[1] == 0 {
		return ex
	}
	return ex % infos[1]
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Other Side of Truth",
		QuestID: 115,
		Items:   []int32{misaLetter, raffortyLetter, pieceOfTablet, reportPiece},
		Bind: script.Bindings{
			script.EventQuestStart: {rafforty},
			script.EventTalked:     {rafforty, misa, kierre, sculpture1, sculpture2, sculpture3, sculpture4},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTimer: onTimer, OnTalk: onTalk},
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
	case strings.EqualFold(e.Name, "32020-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32020-05.htm") || strings.EqualFold(e.Name, "32020-08.htm") || strings.EqualFold(e.Name, "32020-13.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "32020-07.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, misaLetter, 1)
	case strings.EqualFold(e.Name, "32020-11.htm") || strings.EqualFold(e.Name, "32020-12.htm"):
		if st.Cond() == 3 {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "32020-17.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32020-23.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reportPiece, 1)
	case strings.EqualFold(e.Name, "32020-27.htm"):
		if !p.HasItem(pieceOfTablet) {
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = "32020-25.htm"
			finish(s, st, p)
		}
	case strings.EqualFold(e.Name, "32020-28.htm"):
		if !p.HasItem(pieceOfTablet) {
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = "32020-26.htm"
			finish(s, st, p)
		}
	case strings.EqualFold(e.Name, "32018-05.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, raffortyLetter, 1)
	case strings.EqualFold(e.Name, "sculpture-03.htm"):
		infos, ok := sculptureValues(e.NPC)
		if !ok {
			return ""
		}
		ex := st.GetInt("ex")
		if moduloOf(infos, ex) <= infos[2] && (ex == infos[3] || ex == infos[4] || ex == infos[5]) {
			st.Set("ex", strconv.Itoa(int(ex+infos[0])))
			s.GiveItems(p, pieceOfTablet, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "sculpture-04.htm"):
		infos, ok := sculptureValues(e.NPC)
		if !ok {
			return ""
		}
		ex := st.GetInt("ex")
		if moduloOf(infos, ex) <= infos[2] && (ex == infos[3] || ex == infos[4] || ex == infos[5]) {
			st.Set("ex", strconv.Itoa(int(ex+infos[0])))
		}
	case strings.EqualFold(e.Name, "sculpture-06.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		// A suspicious man appears beside the character with a line and
		// leaves a few seconds later with a second one.
		callStranger(s, p, sayManAppears, "despawn_1", 3100*time.Millisecond, 3000*time.Millisecond)
	case strings.EqualFold(e.Name, "32022-02.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, reportPiece, 1)
		callStranger(s, p, sayManReappears, "despawn_2", 5100*time.Millisecond, 5000*time.Millisecond)
	}
	return htmltext
}

// finish trades the piece of the tablet for the reward and ends the quest.
func finish(s *script.Script, st *script.QuestState, p *script.Player) {
	s.TakeItems(p, pieceOfTablet, 1)
	s.RewardItems(p, 57, 60040)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(false)
}

// callStranger spawns the suspicious man 50 east and 50 south of p for
// despawn, saying line, and starts the timer that says his parting line.
func callStranger(s *script.Script, p *script.Player, line int32, timer string, despawn, after time.Duration) {
	at := script.LocOf(p)
	stranger := s.Spawner().AddSpawn(suspiciousMan, script.Loc{X: at.X + 50, Y: at.Y + 50, Z: at.Z, Heading: 0}, false, despawn)
	if stranger == nil {
		return
	}
	stranger.Say(script.NpcString(line))
	s.StartTimer(timer, stranger, nil, after)
}

// onTimer is the suspicious man's parting line.
func onTimer(_ *script.Script, e script.Timer) string {
	switch {
	case strings.EqualFold(e.Name, "despawn_1"):
		e.NPC.Say(script.NpcString(sayManLeaves))
	case strings.EqualFold(e.Name, "despawn_2"):
		e.NPC.Say(script.NpcString(sayManLeaves2))
	}
	return ""
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
		if p.Level() < 53 {
			htmltext = "32020-02.htm"
		} else {
			htmltext = "32020-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case rafforty:
			htmltext = talkRafforty(s, st, p, cond, htmltext)
		case misa:
			switch {
			case cond == 1:
				htmltext = "32018-02.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, misaLetter, 1)
			case cond == 2:
				htmltext = "32018-03.htm"
			case cond == 6:
				htmltext = "32018-04.htm"
			case cond > 6:
				htmltext = "32018-06.htm"
			default:
				htmltext = "32018-01.htm"
			}
		case kierre:
			switch {
			case cond == 8:
				htmltext = "32022-01.htm"
			case cond == 9:
				if !p.HasItem(reportPiece) {
					htmltext = "32022-04.htm"
					s.GiveItems(p, reportPiece, 1)
					s.PlaySound(p, script.SoundItemGet)
				} else {
					htmltext = "32022-03.htm"
				}
			case cond == 11:
				htmltext = "32022-05.htm"
			}
		case sculpture1, sculpture2, sculpture3, sculpture4:
			htmltext = talkSculpture(s, st, p, e.NPC.NpcID(), cond, htmltext)
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkRafforty answers a talk to Rafforty of a started quest at cond,
// keeping htmltext when no step matches.
func talkRafforty(s *script.Script, st *script.QuestState, p *script.Player, cond int32, htmltext string) string {
	switch {
	case cond == 1:
		htmltext = "32020-04.htm"
	case cond == 2:
		htmltext = "32020-06.htm"
	case cond == 3:
		htmltext = "32020-09.htm"
	case cond == 4:
		htmltext = "32020-16.htm"
	case cond == 5:
		htmltext = "32020-18.htm"
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, raffortyLetter, 1)
	case cond == 6:
		if !p.HasItem(raffortyLetter) {
			htmltext = "32020-20.htm"
			s.GiveItems(p, raffortyLetter, 1)
			s.PlaySound(p, script.SoundItemGet)
		} else {
			htmltext = "32020-19.htm"
		}
	case cond == 7:
		htmltext = "32020-19.htm"
	case cond == 8:
		htmltext = "32020-21.htm"
	case cond == 9:
		htmltext = "32020-22.htm"
	case cond == 10:
		htmltext = "32020-24.htm"
	case cond == 11:
		htmltext = "32020-29.htm"
	case cond == 12:
		htmltext = "32020-30.htm"
		finish(s, st, p)
	}
	return htmltext
}

// talkSculpture answers a talk to the sculpture npcID of a started quest
// at cond, keeping htmltext when no step matches.
func talkSculpture(s *script.Script, st *script.QuestState, p *script.Player, npcID, cond int32, htmltext string) string {
	switch {
	case cond == 7:
		infos, _ := npcValues(npcID)
		ex := st.GetInt("ex")
		if moduloOf(infos, ex) <= infos[2] {
			switch ex {
			case infos[3], infos[4], infos[5]:
				htmltext = "sculpture-02.htm"
			case infos[6]:
				htmltext = "sculpture-05.htm"
			default:
				st.Set("ex", strconv.Itoa(int(ex+infos[0])))
				htmltext = "sculpture-01.htm"
			}
		} else {
			htmltext = "sculpture-01a.htm"
		}
	case cond > 7 && cond < 11:
		htmltext = "sculpture-07.htm"
	case cond == 11:
		if !p.HasItem(pieceOfTablet) {
			htmltext = "sculpture-08.htm"
			st.SetCond(12)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, pieceOfTablet, 1)
		} else {
			htmltext = "sculpture-09.htm"
		}
	case cond == 12:
		htmltext = "sculpture-09.htm"
	}
	return htmltext
}
