// Package q412 is the quest Path to a Dark Wizard: a dark mystic earns the
// Jewel of Darkness by collecting the seeds of anger, horror and lunacy from
// Charkeren, Annika and Arkenia.
package q412

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q412_PathToADarkWizard"

// Quest items.
const (
	seedOfAnger     = 1253
	seedOfDespair   = 1254
	seedOfHorror    = 1255
	seedOfLunacy    = 1256
	familyRemains   = 1257
	varikaLiquor    = 1258
	kneeBone        = 1259
	heartOfLunacy   = 1260
	jewelOfDarkness = 1261
	luckyKey        = 1277
	candle          = 1278
	hubScent        = 1279
)

// Npcs.
const (
	varika    = 30421
	charkeren = 30415
	annika    = 30418
	arkenia   = 30419
)

// Monsters.
const (
	familyRemainsMonster = 20015
	kneeBoneMonster1     = 20022
	heartMonster         = 20045
	kneeBoneMonster2     = 20517
	kneeBoneMonster3     = 20518
)

// The classes the quest checks.
const (
	classDarkMystic = 38
	classDarkWizard = 39
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Dark Wizard",
		QuestID: 412,
		Items: []int32{
			seedOfAnger, seedOfDespair, seedOfHorror, seedOfLunacy, familyRemains, varikaLiquor, kneeBone, heartOfLunacy,
			luckyKey, candle, hubScent,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {varika},
			script.EventTalked:     {varika, charkeren, annika, arkenia},
			script.EventMyDying: {
				familyRemainsMonster, kneeBoneMonster1, heartMonster, kneeBoneMonster2, kneeBoneMonster3,
			},
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
	case strings.EqualFold(e.Name, "30421-05.htm"):
		switch {
		case p.ClassID() != classDarkMystic:
			if p.ClassID() == classDarkWizard {
				htmltext = "30421-02a.htm"
			} else {
				htmltext = "30421-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30421-02.htm"
		case p.HasItem(jewelOfDarkness):
			htmltext = "30421-04.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, seedOfDespair, 1)
		}
	case strings.EqualFold(e.Name, "30421-07.htm"):
		switch {
		case p.HasItem(seedOfAnger):
			htmltext = "30421-06.htm"
		case p.HasItem(luckyKey):
			htmltext = "30421-08.htm"
		case p.ItemCount(familyRemains) == 3:
			htmltext = "30421-18.htm"
		}
	case strings.EqualFold(e.Name, "30421-10.htm"):
		switch {
		case p.HasItem(seedOfHorror):
			htmltext = "30421-09.htm"
		case p.ItemCount(kneeBone) == 2:
			htmltext = "30421-19.htm"
		}
	case strings.EqualFold(e.Name, "30421-13.htm"):
		if p.HasItem(seedOfLunacy) {
			htmltext = "30421-12.htm"
		}
	case strings.EqualFold(e.Name, "30415-03.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, luckyKey, 1)
	case strings.EqualFold(e.Name, "30418-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, candle, 1)
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
		htmltext = "30421-01.htm"
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case varika:
			if p.HasItems(seedOfAnger, seedOfHorror, seedOfLunacy) {
				htmltext = "30421-16.htm"
				s.TakeItems(p, seedOfAnger, 1)
				s.TakeItems(p, seedOfDespair, 1)
				s.TakeItems(p, seedOfHorror, 1)
				s.TakeItems(p, seedOfLunacy, 1)
				s.GiveItems(p, jewelOfDarkness, 1)
				s.RewardExpAndSp(p, 3200, 1650)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			} else {
				htmltext = "30421-17.htm"
			}
		case charkeren:
			switch {
			case p.HasItem(seedOfAnger):
				htmltext = "30415-06.htm"
			case !p.HasItem(luckyKey):
				htmltext = "30415-01.htm"
			case p.ItemCount(familyRemains) == 3:
				htmltext = "30415-05.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, familyRemains, -1)
				s.TakeItems(p, luckyKey, 1)
				s.GiveItems(p, seedOfAnger, 1)
			default:
				htmltext = "30415-04.htm"
			}
		case annika:
			switch {
			case p.HasItem(seedOfHorror):
				htmltext = "30418-04.htm"
			case !p.HasItem(candle):
				htmltext = "30418-01.htm"
			case p.ItemCount(kneeBone) == 2:
				htmltext = "30418-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, candle, 1)
				s.TakeItems(p, kneeBone, -1)
				s.GiveItems(p, seedOfHorror, 1)
			default:
				htmltext = "30418-03.htm"
			}
		case arkenia:
			switch {
			case p.HasItem(seedOfLunacy):
				htmltext = "30419-03.htm"
			case !p.HasItem(hubScent):
				htmltext = "30419-01.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, hubScent, 1)
			case p.ItemCount(heartOfLunacy) == 3:
				htmltext = "30419-03.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, heartOfLunacy, -1)
				s.TakeItems(p, hubScent, 1)
				s.GiveItems(p, seedOfLunacy, 1)
			default:
				htmltext = "30419-02.htm"
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
	case familyRemainsMonster:
		if p.HasItem(luckyKey) {
			s.DropItems(p, familyRemains, 1, 3, 500000)
		}
	case kneeBoneMonster1, kneeBoneMonster2, kneeBoneMonster3:
		if p.HasItem(candle) {
			s.DropItems(p, kneeBone, 1, 2, 500000)
		}
	case heartMonster:
		if p.HasItem(hubScent) {
			s.DropItems(p, heartOfLunacy, 1, 3, 500000)
		}
	}
}
