// Package q407 is the quest Path to an Elven Scout: an elven fighter earns
// Reisa's recommendation by gathering the four torn letters from the Ol Mahum
// patrols, a rusted key from the sentries, and the honorary guard of Moretti.
package q407

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q407_PathToAnElvenScout"

// Quest items.
const (
	reisaLetter         = 1207
	priasTornLetter1    = 1208
	priasTornLetter2    = 1209
	priasTornLetter3    = 1210
	priasTornLetter4    = 1211
	morettiHerb         = 1212
	morettiLetter       = 1214
	priasLetter         = 1215
	honoraryGuard       = 1216
	reisaRecommendation = 1217
	rustedKey           = 1293
)

// Npcs.
const (
	reisa   = 30328
	babenco = 30334
	moretti = 30337
	prias   = 30426
)

// Monsters.
const (
	olMahumPatrol = 20053
	olMahumSentry = 27031
)

// The classes the quest checks.
const (
	classElvenFighter = 18
	classElvenScout   = 22
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to an Elven Scout",
		QuestID: 407,
		Items: []int32{
			reisaLetter, priasTornLetter1, priasTornLetter2, priasTornLetter3, priasTornLetter4, morettiHerb, morettiLetter,
			priasLetter, honoraryGuard, rustedKey,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {reisa},
			script.EventTalked:     {reisa, moretti, babenco, prias},
			script.EventMyDying:    {olMahumPatrol, olMahumSentry},
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
	case strings.EqualFold(e.Name, "30328-05.htm"):
		switch {
		case p.ClassID() != classElvenFighter:
			if p.ClassID() == classElvenScout {
				htmltext = "30328-02a.htm"
			} else {
				htmltext = "30328-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30328-03.htm"
		case p.HasItem(reisaRecommendation):
			htmltext = "30328-04.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, reisaLetter, 1)
		}
	case strings.EqualFold(e.Name, "30337-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reisaLetter, -1)
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
		htmltext = "30328-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case reisa:
			switch {
			case cond == 1:
				htmltext = "30328-06.htm"
			case cond > 1 && cond < 8:
				htmltext = "30328-08.htm"
			case cond == 8:
				htmltext = "30328-07.htm"
				s.TakeItems(p, honoraryGuard, -1)
				s.GiveItems(p, reisaRecommendation, 1)
				s.RewardExpAndSp(p, 3200, 1000)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case moretti:
			switch {
			case cond == 1:
				htmltext = "30337-01.htm"
			case cond == 2:
				if !p.HasItem(priasTornLetter1) {
					htmltext = "30337-04.htm"
				} else {
					htmltext = "30337-05.htm"
				}
			case cond == 3:
				htmltext = "30337-06.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, priasTornLetter1, -1)
				s.TakeItems(p, priasTornLetter2, -1)
				s.TakeItems(p, priasTornLetter3, -1)
				s.TakeItems(p, priasTornLetter4, -1)
				s.GiveItems(p, morettiHerb, 1)
				s.GiveItems(p, morettiLetter, 1)
			case cond > 3 && cond < 7:
				htmltext = "30337-09.htm"
			case cond == 7 && p.HasItem(priasLetter):
				htmltext = "30337-07.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, priasLetter, -1)
				s.GiveItems(p, honoraryGuard, 1)
			case cond == 8:
				htmltext = "30337-08.htm"
			}
		case babenco:
			if cond == 2 {
				htmltext = "30334-01.htm"
			}
		case prias:
			switch {
			case cond == 4:
				htmltext = "30426-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 5:
				htmltext = "30426-01.htm"
			case cond == 6:
				htmltext = "30426-02.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, rustedKey, -1)
				s.TakeItems(p, morettiHerb, -1)
				s.TakeItems(p, morettiLetter, -1)
				s.GiveItems(p, priasLetter, 1)
			case cond == 7:
				htmltext = "30426-04.htm"
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
	cond := st.Cond()
	if e.NPC.NpcID() == olMahumPatrol {
		if cond == 2 {
			if !p.HasItem(priasTornLetter1) {
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, priasTornLetter1, 1)
			} else if !p.HasItem(priasTornLetter2) {
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, priasTornLetter2, 1)
			} else if !p.HasItem(priasTornLetter3) {
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, priasTornLetter3, 1)
			} else if !p.HasItem(priasTornLetter4) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, priasTornLetter4, 1)
			}
		}
	} else if (cond == 4 || cond == 5) && s.DropItems(p, rustedKey, 1, 1, 600000) {
		st.SetCond(6)
	}
}
