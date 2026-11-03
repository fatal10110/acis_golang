// Package q401 is the quest Path to a Warrior: a human fighter earns the
// Medallion of Warrior from Auron by collecting rusted bronze swords and
// poison spider legs for Simplon.
package q401

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q401_PathToAWarrior"

// Quest items.
const (
	auronLetter        = 1138
	warriorGuildMark   = 1139
	rustedBronzeSword1 = 1140
	rustedBronzeSword2 = 1141
	rustedBronzeSword3 = 1142
	simplonLetter      = 1143
	poisonSpiderLeg    = 1144
	medallionOfWarrior = 1145
)

// Npcs.
const (
	auron   = 30010
	simplon = 30253
)

// The classes the quest checks.
const (
	classHumanFighter = 0x00
	classWarrior      = 0x01
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Warrior",
		QuestID: 401,
		Items:   []int32{auronLetter, warriorGuildMark, rustedBronzeSword1, rustedBronzeSword2, rustedBronzeSword3, simplonLetter, poisonSpiderLeg},
		Bind: script.Bindings{
			script.EventQuestStart: {auron},
			script.EventTalked:     {auron, simplon},
			script.EventMyDying:    {20035, 20038, 20042, 20043},
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
	case strings.EqualFold(e.Name, "30010-05.htm"):
		switch {
		case p.ClassID() != classHumanFighter:
			if p.ClassID() == classWarrior {
				htmltext = "30010-03.htm"
			} else {
				htmltext = "30010-02b.htm"
			}
		case p.Level() < 19:
			htmltext = "30010-02.htm"
		case p.HasItem(medallionOfWarrior):
			htmltext = "30010-04.htm"
		}
	case strings.EqualFold(e.Name, "30010-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, auronLetter, 1)
	case strings.EqualFold(e.Name, "30253-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, auronLetter, 1)
		s.GiveItems(p, warriorGuildMark, 1)
	case strings.EqualFold(e.Name, "30010-11.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rustedBronzeSword2, 1)
		s.TakeItems(p, simplonLetter, 1)
		s.GiveItems(p, rustedBronzeSword3, 1)
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
		htmltext = "30010-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case auron:
			switch {
			case cond == 1:
				htmltext = "30010-07.htm"
			case cond == 2 || cond == 3:
				htmltext = "30010-08.htm"
			case cond == 4:
				htmltext = "30010-09.htm"
			case cond == 5:
				htmltext = "30010-12.htm"
			case cond == 6:
				htmltext = "30010-13.htm"
				s.TakeItems(p, poisonSpiderLeg, -1)
				s.TakeItems(p, rustedBronzeSword3, 1)
				s.GiveItems(p, medallionOfWarrior, 1)
				s.RewardExpAndSp(p, 3200, 1500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case simplon:
			switch {
			case cond == 1:
				htmltext = "30253-01.htm"
			case cond == 2:
				if !p.HasItem(rustedBronzeSword1) {
					htmltext = "30253-03.htm"
				} else if p.ItemCount(rustedBronzeSword1) <= 9 {
					htmltext = "30253-03b.htm"
				}
			case cond == 3:
				htmltext = "30253-04.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, rustedBronzeSword1, 10)
				s.TakeItems(p, warriorGuildMark, 1)
				s.GiveItems(p, rustedBronzeSword2, 1)
				s.GiveItems(p, simplonLetter, 1)
			case cond == 4:
				htmltext = "30253-05.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case 20035, 20042:
		if st.Cond() == 2 && s.DropItems(p, rustedBronzeSword1, 1, 10, 400000) {
			st.SetCond(3)
		}
	case 20038, 20043:
		if st.Cond() == 5 && p.RightHandItemID() == rustedBronzeSword3 {
			if s.DropItemsAlways(p, poisonSpiderLeg, 1, 20) {
				st.SetCond(6)
			}
		}
	}
}
