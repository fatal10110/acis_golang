// Package q404 is the quest Path to a Human Wizard: a human mystic earns the
// Bead of Season by gathering an earring, a bangle, a necklace and a ring
// from the four spirits.
package q404

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q404_PathToAHumanWizard"

// Quest items.
const (
	mapOfLuster        = 1280
	keyOfFlame         = 1281
	flameEaring        = 1282
	brokenBronzeMirror = 1283
	windFeather        = 1284
	windBangel         = 1285
	ramaDiary          = 1286
	sparklePebble      = 1287
	waterNecklace      = 1288
	rustGoldCoin       = 1289
	redSoil            = 1290
	earthRing          = 1291
	beadOfSeason       = 1292
)

// Npcs.
const (
	parina             = 30391
	earthSnake         = 30409
	wastelandLizardman = 30410
	flameSalamander    = 30411
	windSylph          = 30412
	waterUndine        = 30413
)

// Monsters.
const (
	redBear       = 20021
	ratmanWarrior = 20359
	waterSeer     = 27030
)

// The classes the quest checks.
const (
	classHumanMystic = 10
	classHumanWizard = 11
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Human Wizard",
		QuestID: 404,
		Items: []int32{
			mapOfLuster, keyOfFlame, flameEaring, brokenBronzeMirror, windFeather, windBangel, ramaDiary, sparklePebble,
			waterNecklace, rustGoldCoin, redSoil, earthRing,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {parina},
			script.EventTalked:     {parina, earthSnake, wastelandLizardman, flameSalamander, windSylph, waterUndine},
			script.EventMyDying:    {redBear, ratmanWarrior, waterSeer},
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
	case strings.EqualFold(e.Name, "30391-08.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30410-03.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, brokenBronzeMirror, 1)
		s.GiveItems(p, windFeather, 1)
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
	cond := st.Cond()
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.ClassID() != classHumanMystic:
			if p.ClassID() == classHumanWizard {
				htmltext = "30391-02a.htm"
			} else {
				htmltext = "30391-01.htm"
			}
		case p.Level() < 19:
			htmltext = "30391-02.htm"
		case p.HasItem(beadOfSeason):
			htmltext = "30391-03.htm"
		default:
			htmltext = "30391-04.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case parina:
			if cond < 13 {
				htmltext = "30391-05.htm"
			} else if cond == 13 {
				htmltext = "30391-06.htm"
				s.TakeItems(p, earthRing, 1)
				s.TakeItems(p, flameEaring, 1)
				s.TakeItems(p, waterNecklace, 1)
				s.TakeItems(p, windBangel, 1)
				s.GiveItems(p, beadOfSeason, 1)
				s.RewardExpAndSp(p, 3200, 2020)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case flameSalamander:
			switch {
			case cond == 1:
				htmltext = "30411-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, mapOfLuster, 1)
			case cond == 2:
				htmltext = "30411-02.htm"
			case cond == 3:
				htmltext = "30411-03.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, keyOfFlame, 1)
				s.TakeItems(p, mapOfLuster, 1)
				s.GiveItems(p, flameEaring, 1)
			case cond > 3:
				htmltext = "30411-04.htm"
			}
		case windSylph:
			switch {
			case cond == 4:
				htmltext = "30412-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, brokenBronzeMirror, 1)
			case cond == 5:
				htmltext = "30412-02.htm"
			case cond == 6:
				htmltext = "30412-03.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, windFeather, 1)
				s.GiveItems(p, windBangel, 1)
			case cond > 6:
				htmltext = "30412-04.htm"
			}
		case wastelandLizardman:
			if cond == 5 {
				htmltext = "30410-01.htm"
			} else if cond > 5 {
				htmltext = "30410-04.htm"
			}
		case waterUndine:
			switch {
			case cond == 7:
				htmltext = "30413-01.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, ramaDiary, 1)
			case cond == 8:
				htmltext = "30413-02.htm"
			case cond == 9:
				htmltext = "30413-03.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, ramaDiary, 1)
				s.TakeItems(p, sparklePebble, -1)
				s.GiveItems(p, waterNecklace, 1)
			case cond > 9:
				htmltext = "30413-04.htm"
			}
		case earthSnake:
			switch {
			case cond == 10:
				htmltext = "30409-01.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, rustGoldCoin, 1)
			case cond == 11:
				htmltext = "30409-02.htm"
			case cond == 12:
				htmltext = "30409-03.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, redSoil, 1)
				s.TakeItems(p, rustGoldCoin, 1)
				s.GiveItems(p, earthRing, 1)
			case cond > 12:
				htmltext = "30409-04.htm"
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
	case ratmanWarrior:
		if st.Cond() == 2 && s.DropItems(p, keyOfFlame, 1, 1, 800000) {
			st.SetCond(3)
		}
	case waterSeer:
		if st.Cond() == 8 && s.DropItems(p, sparklePebble, 1, 2, 800000) {
			st.SetCond(9)
		}
	case redBear:
		if st.Cond() == 11 && s.DropItems(p, redSoil, 1, 1, 200000) {
			st.SetCond(12)
		}
	}
}
