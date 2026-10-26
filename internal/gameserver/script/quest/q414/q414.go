// Package q414 is the quest Path to an Orc Raider: an orc fighter brings a
// goblin dwelling map's trail to its end for Karukia, then hunts the betrayer
// for Kasman or Tazeer.
package q414

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q414_PathToAnOrcRaider"

// Quest items.
const (
	greenBlood         = 1578
	goblinDwellingMap  = 1579
	kurukaRatmanTooth  = 1580
	betrayerReport1    = 1589
	betrayerReport2    = 1590
	headOfBetrayer     = 1591
	markOfRaider       = 1592
	timoraOrcHead      = 8544
	goblinTombRaider   = 20320
	kurukaRatmanLeader = 27045
	umbarOrc           = 27054
	timoraOrc          = 27320
)

// Npcs.
const (
	karukia = 30570
	kasman  = 30501
	tazeer  = 31978
)

// The classes the quest checks.
const (
	classOrcFighter = 44
	classOrcRaider  = 45
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path To An Orc Raider",
		QuestID: 414,
		Items: []int32{
			greenBlood, goblinDwellingMap, kurukaRatmanTooth, betrayerReport1, betrayerReport2, headOfBetrayer,
			timoraOrcHead,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {karukia},
			script.EventTalked:     {karukia, kasman, tazeer},
			script.EventMyDying:    {goblinTombRaider, kurukaRatmanLeader, umbarOrc, timoraOrc},
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
	// Karukia
	case strings.EqualFold(e.Name, "30570-05.htm"):
		switch {
		case p.ClassID() != classOrcFighter:
			if p.ClassID() == classOrcRaider {
				htmltext = "30570-02a.htm"
			} else {
				htmltext = "30570-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30570-02.htm"
		case p.HasItem(markOfRaider):
			htmltext = "30570-04.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, goblinDwellingMap, 1)
		}
	case strings.EqualFold(e.Name, "30570-07a.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, goblinDwellingMap, 1)
		s.TakeItems(p, kurukaRatmanTooth, -1)
		s.GiveItems(p, betrayerReport1, 1)
	case strings.EqualFold(e.Name, "30570-07b.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, goblinDwellingMap, 1)
		s.TakeItems(p, kurukaRatmanTooth, -1)
		s.GiveItems(p, betrayerReport2, 1)
	// Tazeer
	case strings.EqualFold(e.Name, "31978-03.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
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
		htmltext = "30570-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case karukia:
			switch {
			case cond == 1:
				htmltext = "30570-06.htm"
			case cond == 2:
				htmltext = "30570-07.htm"
			case cond > 2:
				htmltext = "30570-08.htm"
			}
		case kasman:
			switch {
			case cond == 3:
				if p.HasItem(headOfBetrayer) {
					htmltext = "30501-02.htm"
				} else {
					htmltext = "30501-01.htm"
				}
			case cond == 4:
				htmltext = "30501-03.htm"
				s.TakeItems(p, betrayerReport1, 1)
				s.TakeItems(p, headOfBetrayer, -1)
				s.GiveItems(p, markOfRaider, 1)
				s.RewardExpAndSp(p, 3200, 2360)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case tazeer:
			switch {
			case cond == 5:
				htmltext = "31978-01.htm"
			case cond == 6:
				htmltext = "31978-04.htm"
			case cond == 7:
				htmltext = "31978-05.htm"
				s.TakeItems(p, betrayerReport2, 1)
				s.TakeItems(p, timoraOrcHead, 1)
				s.GiveItems(p, markOfRaider, 1)
				s.RewardExpAndSp(p, 3200, 2360)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
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
	switch e.NPC.NpcID() {
	case goblinTombRaider:
		if cond == 1 {
			if p.ItemCount(greenBlood) <= int32(s.Rnd(20)) {
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, greenBlood, 1)
			} else {
				s.TakeItems(p, greenBlood, -1)
				s.Spawner().AddSpawn(kurukaRatmanLeader, e.NPC, false, 300000*time.Millisecond)
			}
		}
	case kurukaRatmanLeader:
		if cond == 1 && s.DropItemsAlways(p, kurukaRatmanTooth, 1, 10) {
			st.SetCond(2)
		}
	case umbarOrc:
		if cond == 3 && s.DropItems(p, headOfBetrayer, 1, 2, 200000) {
			st.SetCond(4)
		}
	case timoraOrc:
		if cond == 6 && s.DropItems(p, timoraOrcHead, 1, 1, 600000) {
			st.SetCond(7)
		}
	}
}
