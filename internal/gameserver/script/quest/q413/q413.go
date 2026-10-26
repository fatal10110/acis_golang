// Package q413 is the quest Path to a Shillien Oracle: a dark mystic earns
// the Orb of Abyss by gathering bloody runes for Talbot and the ashen bones
// of the penitent for Adonius.
package q413

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q413_PathToAShillienOracle"

// Quest items.
const (
	sidraLetter     = 1262
	blankSheet      = 1263
	bloodyRune      = 1264
	garmielBook     = 1265
	prayerOfAdonius = 1266
	penitentMark    = 1267
	ashenBones      = 1268
	andarielBook    = 1269
	orbOfAbyss      = 1270
)

// Npcs.
const (
	sidra   = 30330
	adonius = 30375
	talbot  = 30377
)

// Monsters.
const runeMonster = 20776

// The classes the quest checks.
const (
	classDarkMystic     = 38
	classShillienOracle = 42
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Shillien Oracle",
		QuestID: 413,
		Items: []int32{
			sidraLetter, blankSheet, bloodyRune, garmielBook, prayerOfAdonius, penitentMark, ashenBones, andarielBook,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {sidra},
			script.EventTalked:     {sidra, adonius, talbot},
			script.EventMyDying:    {runeMonster, 20457, 20458, 20514, 20515},
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
	case strings.EqualFold(e.Name, "30330-05.htm"):
		switch {
		case p.ClassID() != classDarkMystic:
			if p.ClassID() == classShillienOracle {
				htmltext = "30330-02a.htm"
			} else {
				htmltext = "30330-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30330-02.htm"
		case p.HasItem(orbOfAbyss):
			htmltext = "30330-04.htm"
		}
	case strings.EqualFold(e.Name, "30330-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, sidraLetter, 1)
	case strings.EqualFold(e.Name, "30377-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sidraLetter, 1)
		s.GiveItems(p, blankSheet, 5)
	case strings.EqualFold(e.Name, "30375-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, prayerOfAdonius, 1)
		s.GiveItems(p, penitentMark, 1)
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
		htmltext = "30330-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case sidra:
			switch {
			case cond == 1:
				htmltext = "30330-07.htm"
			case cond > 1 && cond < 4:
				htmltext = "30330-08.htm"
			case cond > 3 && cond < 7:
				htmltext = "30330-09.htm"
			case cond == 7:
				htmltext = "30330-10.htm"
				s.TakeItems(p, andarielBook, 1)
				s.TakeItems(p, garmielBook, 1)
				s.GiveItems(p, orbOfAbyss, 1)
				s.RewardExpAndSp(p, 3200, 3120)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case talbot:
			switch {
			case cond == 1:
				htmltext = "30377-01.htm"
			case cond == 2:
				if p.HasItem(bloodyRune) {
					htmltext = "30377-04.htm"
				} else {
					htmltext = "30377-03.htm"
				}
			case cond == 3:
				htmltext = "30377-05.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bloodyRune, -1)
				s.GiveItems(p, garmielBook, 1)
				s.GiveItems(p, prayerOfAdonius, 1)
			case cond > 3 && cond < 7:
				htmltext = "30377-06.htm"
			case cond == 7:
				htmltext = "30377-07.htm"
			}
		case adonius:
			switch {
			case cond == 4:
				htmltext = "30375-01.htm"
			case cond == 5:
				if p.HasItem(ashenBones) {
					htmltext = "30375-05.htm"
				} else {
					htmltext = "30375-06.htm"
				}
			case cond == 6:
				htmltext = "30375-07.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, ashenBones, -1)
				s.TakeItems(p, penitentMark, -1)
				s.GiveItems(p, andarielBook, 1)
			case cond == 7:
				htmltext = "30375-08.htm"
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
	if e.NPC.NpcID() == runeMonster {
		if st.Cond() == 2 {
			s.TakeItems(p, blankSheet, 1)
			if s.DropItemsAlways(p, bloodyRune, 1, 5) {
				st.SetCond(3)
			}
		}
	} else if st.Cond() == 5 && s.DropItemsAlways(p, ashenBones, 1, 10) {
		st.SetCond(6)
	}
}
