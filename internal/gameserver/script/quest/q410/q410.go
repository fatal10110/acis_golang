// Package q410 is the quest Path to a Palus Knight: a dark fighter earns the
// Gaze of Abyss by hunting lycanthropes for their skulls, then spiders and
// trackers for the carapace and silk Morte asks for.
package q410

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q410_PathToAPalusKnight"

// Quest items.
const (
	palusTalisman       = 1237
	lycanthropeSkull    = 1238
	virgilLetter        = 1239
	morteTalisman       = 1240
	predatorCarapace    = 1241
	arachnidTrackerSilk = 1242
	coffinOfEternalRest = 1243
	gazeOfAbyss         = 1244
)

// Npcs.
const (
	kalinta = 30422
	virgil  = 30329
)

// Monsters.
const (
	poisonSpider    = 20038
	arachnidTracker = 20043
	lycanthrope     = 20049
)

// The classes the quest checks.
const (
	classDarkFighter = 31
	classPalusKnight = 32
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Palus Knight",
		QuestID: 410,
		Items: []int32{
			palusTalisman, lycanthropeSkull, virgilLetter, morteTalisman, predatorCarapace, arachnidTrackerSilk,
			coffinOfEternalRest,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {virgil},
			script.EventTalked:     {virgil, kalinta},
			script.EventMyDying:    {poisonSpider, arachnidTracker, lycanthrope},
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
	case strings.EqualFold(e.Name, "30329-05.htm"):
		switch {
		case p.ClassID() != classDarkFighter:
			if p.ClassID() == classPalusKnight {
				htmltext = "30329-02a.htm"
			} else {
				htmltext = "30329-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30329-02.htm"
		case p.HasItem(gazeOfAbyss):
			htmltext = "30329-04.htm"
		}
	case strings.EqualFold(e.Name, "30329-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, palusTalisman, 1)
	case strings.EqualFold(e.Name, "30329-10.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, lycanthropeSkull, -1)
		s.TakeItems(p, palusTalisman, 1)
		s.GiveItems(p, virgilLetter, 1)
	case strings.EqualFold(e.Name, "30422-02.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, virgilLetter, 1)
		s.GiveItems(p, morteTalisman, 1)
	case strings.EqualFold(e.Name, "30422-06.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, arachnidTrackerSilk, -1)
		s.TakeItems(p, morteTalisman, 1)
		s.TakeItems(p, predatorCarapace, -1)
		s.GiveItems(p, coffinOfEternalRest, 1)
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
		htmltext = "30329-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case virgil:
			switch {
			case cond == 1:
				if !p.HasItem(lycanthropeSkull) {
					htmltext = "30329-07.htm"
				} else {
					htmltext = "30329-08.htm"
				}
			case cond == 2:
				htmltext = "30329-09.htm"
			case cond > 2 && cond < 6:
				htmltext = "30329-12.htm"
			case cond == 6:
				htmltext = "30329-11.htm"
				s.TakeItems(p, coffinOfEternalRest, 1)
				s.GiveItems(p, gazeOfAbyss, 1)
				s.RewardExpAndSp(p, 3200, 1500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case kalinta:
			switch cond {
			case 3:
				htmltext = "30422-01.htm"
			case 4:
				if !p.HasItem(arachnidTrackerSilk) || !p.HasItem(predatorCarapace) {
					htmltext = "30422-03.htm"
				} else {
					htmltext = "30422-04.htm"
				}
			case 5:
				htmltext = "30422-05.htm"
			case 6:
				htmltext = "30422-06.htm"
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
	case lycanthrope:
		if st.Cond() == 1 && s.DropItemsAlways(p, lycanthropeSkull, 1, 13) {
			st.SetCond(2)
		}
	case arachnidTracker:
		if st.Cond() == 4 && s.DropItemsAlways(p, arachnidTrackerSilk, 1, 5) && p.HasItem(predatorCarapace) {
			st.SetCond(5)
		}
	case poisonSpider:
		if st.Cond() == 4 && s.DropItemsAlways(p, predatorCarapace, 1, 1) && p.ItemCount(arachnidTrackerSilk) == 5 {
			st.SetCond(5)
		}
	}
}
