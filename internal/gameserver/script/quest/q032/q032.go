// Package q032 is the quest An Obvious Lie: Maximilian sends the player to
// Gentler, who wants medicinal herbs from alligators and spirit ore for
// Miki the Cat's lie, then sews a pair of animal ears from suede and thread.
package q032

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q032_AnObviousLie"

// Items.
const (
	suede         = 1866
	thread        = 1868
	spiritOre     = 3031
	questMap      = 7165
	medicinalHerb = 7166
)

// Rewards.
const (
	catEars    = 6843
	racoonEars = 7680
	rabbitEars = 7683
)

// NPCs.
const (
	gentler    = 30094
	maximilian = 30120
	mikiTheCat = 31706
)

// alligator drops the medicinal herbs.
const alligator = 20135

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "An Obvious Lie",
		QuestID: 32,
		Items:   []int32{questMap, medicinalHerb},
		Bind: script.Bindings{
			script.EventQuestStart: {maximilian},
			script.EventTalked:     {maximilian, gentler, mikiTheCat},
			script.EventMyDying:    {alligator},
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
	case strings.EqualFold(e.Name, "30120-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30094-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "31706-1.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "30094-4.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, medicinalHerb, 20)
	case strings.EqualFold(e.Name, "30094-7.htm"):
		if p.ItemCount(spiritOre) < 500 {
			htmltext = "30094-5.htm"
		} else {
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, spiritOre, 500)
		}
	case strings.EqualFold(e.Name, "31706-4.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30094-10.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30094-13.htm"):
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "cat"):
		htmltext = sewEars(s, st, p, catEars)
	case strings.EqualFold(e.Name, "racoon"):
		htmltext = sewEars(s, st, p, racoonEars)
	case strings.EqualFold(e.Name, "rabbit"):
		htmltext = sewEars(s, st, p, rabbitEars)
	}
	return htmltext
}

// sewEars trades the suede and thread for the chosen ears and ends the
// quest, or answers that the materials are short.
func sewEars(s *script.Script, st *script.QuestState, p *script.Player, ears int32) string {
	if p.ItemCount(thread) < 1000 || p.ItemCount(suede) < 500 {
		return "30094-11.htm"
	}
	s.TakeItems(p, suede, 500)
	s.TakeItems(p, thread, 1000)
	s.GiveItems(p, ears, 1)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(false)
	return "30094-14.htm"
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
		if p.Level() < 45 {
			htmltext = "30120-0a.htm"
		} else {
			htmltext = "30120-0.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case maximilian:
			htmltext = "30120-2.htm"
		case gentler:
			switch {
			case cond == 1:
				htmltext = "30094-0.htm"
			case cond == 2 || cond == 3:
				htmltext = "30094-2.htm"
			case cond == 4:
				htmltext = "30094-3.htm"
			case cond == 5:
				if p.ItemCount(spiritOre) < 500 {
					htmltext = "30094-5.htm"
				} else {
					htmltext = "30094-6.htm"
				}
			case cond == 6:
				htmltext = "30094-8.htm"
			case cond == 7:
				htmltext = "30094-9.htm"
			case cond == 8:
				if p.ItemCount(thread) < 1000 || p.ItemCount(suede) < 500 {
					htmltext = "30094-11.htm"
				} else {
					htmltext = "30094-12.htm"
				}
			}
		case mikiTheCat:
			switch {
			case cond == 2:
				htmltext = "31706-0.htm"
			case cond > 2 && cond < 6:
				htmltext = "31706-2.htm"
			case cond == 6:
				htmltext = "31706-3.htm"
			case cond > 6:
				htmltext = "31706-5.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 3)
	if st == nil {
		return
	}
	if s.DropItemsAlways(p, medicinalHerb, 1, 20) {
		st.SetCond(4)
	}
}
