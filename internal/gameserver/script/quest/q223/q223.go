// Package q223 is the quest Test of the Champion, a second class-change
// test for warriors and orc raiders: Ascalon sends the traveller to Mason
// for the Bloody Axe heads, to Groot for harpy eggs, medusa venom and
// windsus bile, and to Mouen for road ratman heads and Leto lizardman
// fangs, for the Mark of Champion.
package q223

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q223_TestOfTheChampion"

// Items.
const (
	ascalonLetter1    = 3277
	masonLetter       = 3278
	ironRoseRing      = 3279
	ascalonLetter2    = 3280
	whiteRoseInsignia = 3281
	grootLetter       = 3282
	ascalonLetter3    = 3283
	mouenOrder1       = 3284
	mouenOrder2       = 3285
	mouenLetter       = 3286
	harpyEgg          = 3287
	medusaVenom       = 3288
	windsusBile       = 3289
	bloodyAxeHead     = 3290
	roadRatmanHead    = 3291
	letoLizardmanFang = 3292
)

// Rewards.
const markOfChampion = 3276

// NPCs.
const (
	ascalon = 30624
	groot   = 30093
	mouen   = 30196
	mason   = 30625
)

// Monsters.
const (
	harpy                 = 20145
	harpyMatriarch        = 27088
	medusa                = 20158
	windsus               = 20553
	roadCollector         = 27089
	roadScavenger         = 20551
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
	bloodyAxeElite        = 20780
)

// The classes the test is open to.
const (
	classWarrior   = 1
	classOrcRaider = 45
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test of the Champion",
		QuestID: 223,
		Items: []int32{
			masonLetter, medusaVenom, windsusBile, whiteRoseInsignia, harpyEgg, grootLetter, mouenLetter,
			ascalonLetter1, ironRoseRing, bloodyAxeHead, ascalonLetter2, ascalonLetter3, mouenOrder1,
			roadRatmanHead, mouenOrder2, letoLizardmanFang,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {ascalon},
			script.EventTalked:     {ascalon, groot, mouen, mason},
			script.EventAttacked:   {harpy, roadScavenger},
			script.EventMyDying: {
				harpy, medusa, harpyMatriarch, roadCollector, roadScavenger, windsus, letoLizardman,
				letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman,
				letoLizardmanOverlord, bloodyAxeElite,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch e.Name {
	case "30624-06.htm":
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ascalonLetter1, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30624-06a.htm"
		}
	case "30624-10.htm":
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, masonLetter, 1)
		s.GiveItems(p, ascalonLetter2, 1)
	case "30624-14.htm":
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, grootLetter, 1)
		s.GiveItems(p, ascalonLetter3, 1)
	case "30625-03.htm":
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, ascalonLetter1, 1)
		s.GiveItems(p, ironRoseRing, 1)
	case "30093-02.htm":
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, ascalonLetter2, 1)
		s.GiveItems(p, whiteRoseInsignia, 1)
	case "30196-03.htm":
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, ascalonLetter3, 1)
		s.GiveItems(p, mouenOrder1, 1)
	case "30196-06.htm":
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mouenOrder1, 1)
		s.TakeItems(p, roadRatmanHead, 1)
		s.GiveItems(p, mouenOrder2, 1)
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
		switch class := p.ClassID(); {
		case class != classWarrior && class != classOrcRaider:
			htmltext = "30624-01.htm"
		case p.Level() < 39:
			htmltext = "30624-02.htm"
		case class == classWarrior:
			htmltext = "30624-03.htm"
		default:
			htmltext = "30624-04.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case ascalon:
			switch {
			case cond == 1:
				htmltext = "30624-07.htm"
			case cond < 4:
				htmltext = "30624-08.htm"
			case cond == 4:
				htmltext = "30624-09.htm"
			case cond == 5:
				htmltext = "30624-11.htm"
			case cond > 5 && cond < 8:
				htmltext = "30624-12.htm"
			case cond == 8:
				htmltext = "30624-13.htm"
			case cond == 9:
				htmltext = "30624-15.htm"
			case cond > 9 && cond < 14:
				htmltext = "30624-16.htm"
			case cond == 14:
				htmltext = "30624-17.htm"
				s.TakeItems(p, mouenLetter, 1)
				s.GiveItems(p, markOfChampion, 1)
				s.RewardExpAndSp(p, 117454, 25000)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case mason:
			switch {
			case cond == 1:
				htmltext = "30625-01.htm"
			case cond == 2:
				htmltext = "30625-04.htm"
			case cond == 3:
				htmltext = "30625-05.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bloodyAxeHead, -1)
				s.TakeItems(p, ironRoseRing, 1)
				s.GiveItems(p, masonLetter, 1)
			case cond == 4:
				htmltext = "30625-06.htm"
			case cond > 4:
				htmltext = "30625-07.htm"
			}
		case groot:
			switch {
			case cond == 5:
				htmltext = "30093-01.htm"
			case cond == 6:
				htmltext = "30093-03.htm"
			case cond == 7:
				htmltext = "30093-04.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, whiteRoseInsignia, 1)
				s.TakeItems(p, harpyEgg, -1)
				s.TakeItems(p, medusaVenom, -1)
				s.TakeItems(p, windsusBile, -1)
				s.GiveItems(p, grootLetter, 1)
			case cond == 8:
				htmltext = "30093-05.htm"
			case cond > 8:
				htmltext = "30093-06.htm"
			}
		case mouen:
			switch {
			case cond == 9:
				htmltext = "30196-01.htm"
			case cond == 10:
				htmltext = "30196-04.htm"
			case cond == 11:
				htmltext = "30196-05.htm"
			case cond == 12:
				htmltext = "30196-07.htm"
			case cond == 13:
				htmltext = "30196-08.htm"
				st.SetCond(14)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, letoLizardmanFang, -1)
				s.TakeItems(p, mouenOrder2, 1)
				s.GiveItems(p, mouenLetter, 1)
			case cond > 13:
				htmltext = "30196-09.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case harpy:
		// A harpy may call one or two matriarchs.
		if st.Cond() == 6 && s.RndBool() && !e.NPC.IsScriptValue(1) {
			callHelpers(s, e, harpyMatriarch)
		}
	case roadScavenger:
		// A road scavenger may call one or two road collectors.
		if st.Cond() == 10 && s.RndBool() && !e.NPC.IsScriptValue(1) {
			callHelpers(s, e, roadCollector)
		}
	}
}

// callHelpers spawns helpers of npcID around the attacked NPC, set on its
// attacker, and marks the NPC so it calls no more. The loop bound is drawn
// again before each spawn: the first spawn always happens, a second one
// when its draw is 7 or more of 10.
func callHelpers(s *script.Script, e script.Attacked, npcID int32) {
	for i := 1; i < helperBound(s); i++ {
		if helper := s.Spawner().AddSpawn(npcID, e.NPC, true, 0); helper != nil {
			helper.AddAttackDesire(e.Attacker, 200)
		}
	}
	e.NPC.SetScriptValue(1)
}

// helperBound draws the bound of callHelpers' loop: 2 with a draw under 7
// of 10, else 3.
func helperBound(s *script.Script) int {
	if s.Rnd(10) < 7 {
		return 2
	}
	return 3
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch npcID {
	case bloodyAxeElite:
		if st.Cond() == 2 && s.DropItemsAlways(p, bloodyAxeHead, 1, 100) {
			st.SetCond(3)
		}
	case harpy, harpyMatriarch:
		if st.Cond() == 6 && s.DropItems(p, harpyEgg, 1, 30, 500000) && p.ItemCount(medusaVenom) == 30 && p.ItemCount(windsusBile) == 30 {
			st.SetCond(7)
		}
	case medusa:
		if st.Cond() == 6 && s.DropItems(p, medusaVenom, 1, 30, 500000) && p.ItemCount(harpyEgg) == 30 && p.ItemCount(windsusBile) == 30 {
			st.SetCond(7)
		}
	case windsus:
		if st.Cond() == 6 && s.DropItems(p, windsusBile, 1, 30, 500000) && p.ItemCount(harpyEgg) == 30 && p.ItemCount(medusaVenom) == 30 {
			st.SetCond(7)
		}
	case roadCollector, roadScavenger:
		if st.Cond() == 10 && s.DropItemsAlways(p, roadRatmanHead, 1, 100) {
			st.SetCond(11)
		}
	case letoLizardman, letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord:
		if st.Cond() == 12 && s.DropItems(p, letoLizardmanFang, 1, 100, 500000+(100000*(npcID-20577))) {
			st.SetCond(13)
		}
	}
}
