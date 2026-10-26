// Package q228 is the quest Test of Magus, a second class-change test for
// wizards: Rukal sends the traveller to Parina and Casian for the three
// golden seeds of the singing flowers, then to the four elementals of
// water, fire, wind and earth for their tones, for the Mark of Magus.
package q228

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q228_TestOfMagus"

// Items.
const (
	rukalLetter        = 2841
	parinaLetter       = 2842
	lilacCharm         = 2843
	goldenSeed1        = 2844
	goldenSeed2        = 2845
	goldenSeed3        = 2846
	scoreOfElements    = 2847
	dazzlingDrop       = 2848
	flameCrystal       = 2849
	harpyFeather       = 2850
	wyrmWingbone       = 2851
	windsusMane        = 2852
	enMonstereyeShell  = 2853
	enStonegolemPowder = 2854
	enIrongolemScrap   = 2855
	toneOfWater        = 2856
	toneOfFire         = 2857
	toneOfWind         = 2858
	toneOfEarth        = 2859
	salamanderCharm    = 2860
	sylphCharm         = 2861
	undineCharm        = 2862
	serpentCharm       = 2863
)

// Rewards.
const markOfMagus = 2840

// NPCs.
const (
	parina          = 30391
	earthSnake      = 30409
	flameSalamander = 30411
	windSylph       = 30412
	waterUndine     = 30413
	casian          = 30612
	rukal           = 30629
)

// Monsters.
const (
	harpy                  = 20145
	marshStakato           = 20157
	wyrm                   = 20176
	marshStakatoWorker     = 20230
	toadLord               = 20231
	marshStakatoSoldier    = 20232
	marshStakatoDrone      = 20234
	windsus                = 20553
	enchantedMonstereye    = 20564
	enchantedStoneGolem    = 20565
	enchantedIronGolem     = 20566
	singingFlowerPhantasm  = 27095
	singingFlowerNightmare = 27096
	singingFlowerDarkling  = 27097
	ghostFire              = 27098
)

// The NpcStrings the singing flowers cry out as they die.
const (
	phantasmCry  = 22819
	nightmareCry = 22820
	darklingCry  = 22821
)

// The classes the test is open to.
const (
	classHumanWizard = 11
	classElvenWizard = 26
	classDarkWizard  = 38
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test Of Magus",
		QuestID: 228,
		Items: []int32{
			rukalLetter, parinaLetter, lilacCharm, goldenSeed1, goldenSeed2, goldenSeed3, scoreOfElements, dazzlingDrop,
			flameCrystal, harpyFeather, wyrmWingbone, windsusMane, enMonstereyeShell, enStonegolemPowder,
			enIrongolemScrap, toneOfWater, toneOfFire, toneOfWind, toneOfEarth, salamanderCharm, sylphCharm,
			undineCharm, serpentCharm,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {rukal},
			script.EventTalked:     {parina, earthSnake, flameSalamander, windSylph, waterUndine, casian, rukal},
			script.EventMyDying: {
				harpy, marshStakato, wyrm, marshStakatoWorker, toadLord, marshStakatoSoldier, marshStakatoDrone, windsus,
				enchantedMonstereye, enchantedStoneGolem, enchantedIronGolem, singingFlowerPhantasm,
				singingFlowerNightmare, singingFlowerDarkling, ghostFire,
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
	// Rukal.
	case strings.EqualFold(e.Name, "30629-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, rukalLetter, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30629-04a.htm"
		}
	case strings.EqualFold(e.Name, "30629-10.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, goldenSeed1, 1)
		s.TakeItems(p, goldenSeed2, 1)
		s.TakeItems(p, goldenSeed3, 1)
		s.TakeItems(p, lilacCharm, 1)
		s.GiveItems(p, scoreOfElements, 1)
	// Parina.
	case strings.EqualFold(e.Name, "30391-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rukalLetter, 1)
		s.GiveItems(p, parinaLetter, 1)
	// Casian.
	case strings.EqualFold(e.Name, "30612-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, parinaLetter, 1)
		s.GiveItems(p, lilacCharm, 1)
	// Wind sylph.
	case strings.EqualFold(e.Name, "30412-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, sylphCharm, 1)
	// Earth snake.
	case strings.EqualFold(e.Name, "30409-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, serpentCharm, 1)
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
		case class != classHumanWizard && class != classElvenWizard && class != classDarkWizard:
			htmltext = "30629-01.htm"
		case p.Level() < 39:
			htmltext = "30629-02.htm"
		default:
			htmltext = "30629-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case rukal:
		switch cond {
		case 1:
			htmltext = "30629-05.htm"
		case 2:
			htmltext = "30629-06.htm"
		case 3:
			htmltext = "30629-07.htm"
		case 4:
			htmltext = "30629-08.htm"
		case 5:
			htmltext = "30629-11.htm"
		case 6:
			htmltext = "30629-12.htm"
			s.TakeItems(p, scoreOfElements, 1)
			s.TakeItems(p, toneOfEarth, 1)
			s.TakeItems(p, toneOfFire, 1)
			s.TakeItems(p, toneOfWater, 1)
			s.TakeItems(p, toneOfWind, 1)
			s.GiveItems(p, markOfMagus, 1)
			s.RewardExpAndSp(p, 139039, 40000)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case parina:
		switch {
		case cond == 1:
			htmltext = "30391-01.htm"
		case cond == 2:
			htmltext = "30391-03.htm"
		case cond == 3 || cond == 4:
			htmltext = "30391-04.htm"
		case cond > 4:
			htmltext = "30391-05.htm"
		}
	case casian:
		switch {
		case cond == 2:
			htmltext = "30612-01.htm"
		case cond == 3:
			htmltext = "30612-03.htm"
		case cond == 4:
			htmltext = "30612-04.htm"
		case cond > 4:
			htmltext = "30612-05.htm"
		}
	case waterUndine:
		switch cond {
		case 5:
			switch {
			case p.HasItem(undineCharm):
				if p.ItemCount(dazzlingDrop) < 20 {
					htmltext = "30413-02.htm"
				} else {
					htmltext = "30413-03.htm"
					s.TakeItems(p, dazzlingDrop, 20)
					s.TakeItems(p, undineCharm, 1)
					s.GiveItems(p, toneOfWater, 1)
					if p.HasItems(toneOfFire, toneOfWind, toneOfEarth) {
						st.SetCond(6)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				}
			case !p.HasItem(toneOfWater):
				htmltext = "30413-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, undineCharm, 1)
			default:
				htmltext = "30413-04.htm"
			}
		case 6:
			htmltext = "30413-04.htm"
		}
	case flameSalamander:
		switch cond {
		case 5:
			switch {
			case p.HasItem(salamanderCharm):
				if p.ItemCount(flameCrystal) < 5 {
					htmltext = "30411-02.htm"
				} else {
					htmltext = "30411-03.htm"
					s.TakeItems(p, flameCrystal, 5)
					s.TakeItems(p, salamanderCharm, 1)
					s.GiveItems(p, toneOfFire, 1)
					if p.HasItems(toneOfWater, toneOfWind, toneOfEarth) {
						st.SetCond(6)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				}
			case !p.HasItem(toneOfFire):
				htmltext = "30411-01.htm"
				s.GiveItems(p, salamanderCharm, 1)
			default:
				htmltext = "30411-04.htm"
			}
		case 6:
			htmltext = "30411-04.htm"
		}
	case windSylph:
		switch cond {
		case 5:
			switch {
			case p.HasItem(sylphCharm):
				if p.ItemCount(harpyFeather)+p.ItemCount(wyrmWingbone)+p.ItemCount(windsusMane) < 40 {
					htmltext = "30412-03.htm"
				} else {
					htmltext = "30412-04.htm"
					s.TakeItems(p, harpyFeather, 20)
					s.TakeItems(p, sylphCharm, 1)
					s.TakeItems(p, windsusMane, 10)
					s.TakeItems(p, wyrmWingbone, 10)
					s.GiveItems(p, toneOfWind, 1)
					if p.HasItems(toneOfWater, toneOfFire, toneOfEarth) {
						st.SetCond(6)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				}
			case !p.HasItem(toneOfWind):
				htmltext = "30412-01.htm"
			default:
				htmltext = "30412-05.htm"
			}
		case 6:
			htmltext = "30412-05.htm"
		}
	case earthSnake:
		switch cond {
		case 5:
			switch {
			case p.HasItem(serpentCharm):
				if p.ItemCount(enMonstereyeShell)+p.ItemCount(enStonegolemPowder)+p.ItemCount(enIrongolemScrap) < 30 {
					htmltext = "30409-04.htm"
				} else {
					htmltext = "30409-05.htm"
					s.TakeItems(p, enIrongolemScrap, 10)
					s.TakeItems(p, enMonstereyeShell, 10)
					s.TakeItems(p, enStonegolemPowder, 10)
					s.TakeItems(p, serpentCharm, 1)
					s.GiveItems(p, toneOfEarth, 1)
					if p.HasItems(toneOfWater, toneOfFire, toneOfWind) {
						st.SetCond(6)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				}
			case !p.HasItem(toneOfEarth):
				htmltext = "30409-01.htm"
			default:
				htmltext = "30409-06.htm"
			}
		case 6:
			htmltext = "30409-06.htm"
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
	switch st.Cond() {
	case 3:
		switch e.NPC.NpcID() {
		case singingFlowerPhantasm:
			if !p.HasItem(goldenSeed1) {
				e.NPC.Say(script.NpcString(phantasmCry))
				s.DropItemsAlways(p, goldenSeed1, 1, 1)
				if p.HasItems(goldenSeed2, goldenSeed3) {
					st.SetCond(4)
				}
			}
		case singingFlowerNightmare:
			if !p.HasItem(goldenSeed2) {
				e.NPC.Say(script.NpcString(nightmareCry))
				s.DropItemsAlways(p, goldenSeed2, 1, 1)
				if p.HasItems(goldenSeed1, goldenSeed3) {
					st.SetCond(4)
				}
			}
		case singingFlowerDarkling:
			if !p.HasItem(goldenSeed3) {
				e.NPC.Say(script.NpcString(darklingCry))
				s.DropItemsAlways(p, goldenSeed3, 1, 1)
				if p.HasItems(goldenSeed1, goldenSeed2) {
					st.SetCond(4)
				}
			}
		}
	case 5:
		switch e.NPC.NpcID() {
		case ghostFire:
			if p.HasItem(salamanderCharm) {
				s.DropItems(p, flameCrystal, 1, 5, 500000)
			}
		case toadLord, marshStakato, marshStakatoWorker:
			if p.HasItem(undineCharm) {
				s.DropItems(p, dazzlingDrop, 1, 20, 300000)
			}
		case marshStakatoSoldier:
			if p.HasItem(undineCharm) {
				s.DropItems(p, dazzlingDrop, 1, 20, 400000)
			}
		case marshStakatoDrone:
			if p.HasItem(undineCharm) {
				s.DropItems(p, dazzlingDrop, 1, 20, 500000)
			}
		case harpy:
			if p.HasItem(sylphCharm) {
				s.DropItemsAlways(p, harpyFeather, 1, 20)
			}
		case wyrm:
			if p.HasItem(sylphCharm) {
				s.DropItems(p, wyrmWingbone, 1, 10, 500000)
			}
		case windsus:
			if p.HasItem(sylphCharm) {
				s.DropItems(p, windsusMane, 1, 10, 500000)
			}
		case enchantedMonstereye:
			if p.HasItem(serpentCharm) {
				s.DropItemsAlways(p, enMonstereyeShell, 1, 10)
			}
		case enchantedStoneGolem:
			if p.HasItem(serpentCharm) {
				s.DropItemsAlways(p, enStonegolemPowder, 1, 10)
			}
		case enchantedIronGolem:
			if p.HasItem(serpentCharm) {
				s.DropItemsAlways(p, enIrongolemScrap, 1, 10)
			}
		}
	}
}
