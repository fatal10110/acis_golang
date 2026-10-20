// Package q217 is the quest Testimony of Trust, a second class-change
// testimony for humans: Hollint sends the traveller for the trust of the
// elves, the dark elves, the dwarves and the orcs, then to Biotin for the
// Mark of Trust.
package q217

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q217_TestimonyOfTrust"

// Items.
const (
	letterToElf             = 2735
	letterToDarkElf         = 2736
	letterToDwarf           = 2737
	letterToOrc             = 2738
	letterToSeresin         = 2739
	scrollOfDarkElfTrust    = 2740
	scrollOfElfTrust        = 2741
	scrollOfDwarfTrust      = 2742
	scrollOfOrcTrust        = 2743
	recommendationOfHollint = 2744
	orderOfAsterios         = 2745
	breathOfWinds           = 2746
	seedOfVerdure           = 2747
	letterFromThifiell      = 2748
	bloodGuardianBasilik    = 2749
	giantAphid              = 2750
	stakatoFluids           = 2751
	basilikPlasma           = 2752
	honeyDew                = 2753
	stakatoIchor            = 2754
	orderOfClayton          = 2755
	parasiteOfLota          = 2756
	letterToManakia         = 2757
	letterOfManakia         = 2758
	letterToNikola          = 2759
	orderOfNikola           = 2760
	heartstoneOfPorta       = 2761
)

// Rewards.
const markOfTrust = 2734

// NPCs.
const (
	hollint  = 30191
	asterios = 30154
	thifiell = 30358
	clayton  = 30464
	seresin  = 30657
	kakai    = 30565
	manakia  = 30515
	lockirin = 30531
	nikola   = 30621
	biotin   = 30031
)

// Monsters.
const (
	dryad               = 20013
	dryadElder          = 20019
	lirein              = 20036
	lireinElder         = 20044
	acteaOfVerdantWilds = 27121
	luellOfZephyrWinds  = 27120
	guardianBasilik     = 20550
	antRecruit          = 20082
	antPatrol           = 20084
	antGuard            = 20086
	antSoldier          = 20087
	antWarriorCaptain   = 20088
	marshStakato        = 20157
	marshStakatoWorker  = 20230
	marshStakatoSoldier = 20232
	marshStakatoDrone   = 20234
	windsus             = 20553
	porta               = 20213
)

// guardianDespawn is how long a guardian a dryad or a lirein calls stays.
const guardianDespawn = 200000 * time.Millisecond

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Testimony of Trust",
		QuestID: 217,
		Items: []int32{
			letterToElf, letterToDarkElf, letterToDwarf, letterToOrc, letterToSeresin, scrollOfDarkElfTrust,
			scrollOfElfTrust, scrollOfDwarfTrust, scrollOfOrcTrust, recommendationOfHollint, orderOfAsterios,
			breathOfWinds, seedOfVerdure, letterFromThifiell, bloodGuardianBasilik, giantAphid, stakatoFluids,
			basilikPlasma, honeyDew, stakatoIchor, orderOfClayton, parasiteOfLota, letterToManakia,
			letterOfManakia, letterToNikola, orderOfNikola, heartstoneOfPorta,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {hollint},
			script.EventTalked:     {hollint, asterios, thifiell, clayton, seresin, kakai, manakia, lockirin, nikola, biotin},
			script.EventMyDying: {
				dryad, dryadElder, lirein, lireinElder, acteaOfVerdantWilds, luellOfZephyrWinds, guardianBasilik,
				antRecruit, antPatrol, antGuard, antSoldier, antWarriorCaptain, marshStakato, marshStakatoWorker,
				marshStakatoSoldier, marshStakatoDrone, windsus, porta,
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
	case strings.EqualFold(e.Name, "30191-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, letterToElf, 1)
		s.GiveItems(p, letterToDarkElf, 1)
		if secondclass.GiveDimensionalDiamonds37(s, p) {
			htmltext = "30191-04a.htm"
		}
	case strings.EqualFold(e.Name, "30154-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToElf, 1)
		s.GiveItems(p, orderOfAsterios, 1)
	case strings.EqualFold(e.Name, "30358-02.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToDarkElf, 1)
		s.GiveItems(p, letterFromThifiell, 1)
	case strings.EqualFold(e.Name, "30515-02.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToManakia, 1)
	case strings.EqualFold(e.Name, "30531-02.htm"):
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToDwarf, 1)
		s.GiveItems(p, letterToNikola, 1)
	case strings.EqualFold(e.Name, "30565-02.htm"):
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToOrc, 1)
		s.GiveItems(p, letterToManakia, 1)
	case strings.EqualFold(e.Name, "30621-02.htm"):
		st.SetCond(19)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterToNikola, 1)
		s.GiveItems(p, orderOfNikola, 1)
	case strings.EqualFold(e.Name, "30657-03.htm"):
		if p.Level() < 38 {
			htmltext = "30657-02.htm"
			if st.Cond() == 10 {
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
			}
		} else {
			st.SetCond(12)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, letterToSeresin, 1)
			s.GiveItems(p, letterToDwarf, 1)
			s.GiveItems(p, letterToOrc, 1)
		}
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
		switch {
		case p.ClassLevel() != 1:
			htmltext = "30191-01a.htm"
		case p.Race() != player.RaceHuman:
			htmltext = "30191-02.htm"
		case p.Level() < 37:
			htmltext = "30191-01.htm"
		default:
			htmltext = "30191-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case hollint:
			switch {
			case cond < 9:
				htmltext = "30191-08.htm"
			case cond == 9:
				htmltext = "30191-05.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, scrollOfDarkElfTrust, 1)
				s.TakeItems(p, scrollOfElfTrust, 1)
				s.GiveItems(p, letterToSeresin, 1)
			case cond > 9 && cond < 22:
				htmltext = "30191-09.htm"
			case cond == 22:
				htmltext = "30191-06.htm"
				st.SetCond(23)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, scrollOfDwarfTrust, 1)
				s.TakeItems(p, scrollOfOrcTrust, 1)
				s.GiveItems(p, recommendationOfHollint, 1)
			case cond == 23:
				htmltext = "30191-07.htm"
			}
		case asterios:
			switch {
			case cond == 1:
				htmltext = "30154-01.htm"
			case cond == 2:
				htmltext = "30154-04.htm"
			case cond == 3:
				htmltext = "30154-05.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, breathOfWinds, 1)
				s.TakeItems(p, seedOfVerdure, 1)
				s.TakeItems(p, orderOfAsterios, 1)
				s.GiveItems(p, scrollOfElfTrust, 1)
			case cond > 3:
				htmltext = "30154-06.htm"
			}
		case thifiell:
			switch {
			case cond == 4:
				htmltext = "30358-01.htm"
			case cond > 4 && cond < 8:
				htmltext = "30358-05.htm"
			case cond == 8:
				htmltext = "30358-03.htm"
				st.SetCond(9)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, basilikPlasma, 1)
				s.TakeItems(p, honeyDew, 1)
				s.TakeItems(p, stakatoIchor, 1)
				s.GiveItems(p, scrollOfDarkElfTrust, 1)
			case cond > 8:
				htmltext = "30358-04.htm"
			}
		case clayton:
			switch {
			case cond == 5:
				htmltext = "30464-01.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, letterFromThifiell, 1)
				s.GiveItems(p, orderOfClayton, 1)
			case cond == 6:
				htmltext = "30464-02.htm"
			case cond > 6:
				htmltext = "30464-03.htm"
				if cond == 7 {
					st.SetCond(8)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, orderOfClayton, 1)
				}
			}
		case seresin:
			switch {
			case cond == 10 || cond == 11:
				htmltext = "30657-01.htm"
			case cond > 11 && cond < 22:
				htmltext = "30657-04.htm"
			case cond == 22:
				htmltext = "30657-05.htm"
			}
		case kakai:
			switch {
			case cond == 12:
				htmltext = "30565-01.htm"
			case cond > 12 && cond < 16:
				htmltext = "30565-03.htm"
			case cond == 16:
				htmltext = "30565-04.htm"
				st.SetCond(17)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, letterOfManakia, 1)
				s.GiveItems(p, scrollOfOrcTrust, 1)
			case cond > 16:
				htmltext = "30565-05.htm"
			}
		case manakia:
			switch {
			case cond == 13:
				htmltext = "30515-01.htm"
			case cond == 14:
				htmltext = "30515-03.htm"
			case cond == 15:
				htmltext = "30515-04.htm"
				st.SetCond(16)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, parasiteOfLota, -1)
				s.GiveItems(p, letterOfManakia, 1)
			case cond > 15:
				htmltext = "30515-05.htm"
			}
		case lockirin:
			switch {
			case cond == 17:
				htmltext = "30531-01.htm"
			case cond > 17 && cond < 21:
				htmltext = "30531-03.htm"
			case cond == 21:
				htmltext = "30531-04.htm"
				st.SetCond(22)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, scrollOfDwarfTrust, 1)
			case cond == 22:
				htmltext = "30531-05.htm"
			}
		case nikola:
			switch {
			case cond == 18:
				htmltext = "30621-01.htm"
			case cond == 19:
				htmltext = "30621-03.htm"
			case cond == 20:
				htmltext = "30621-04.htm"
				st.SetCond(21)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, heartstoneOfPorta, -1)
				s.TakeItems(p, orderOfNikola, 1)
			case cond > 20:
				htmltext = "30621-05.htm"
			}
		case biotin:
			if cond == 23 {
				htmltext = "30031-01.htm"
				s.TakeItems(p, recommendationOfHollint, 1)
				s.GiveItems(p, markOfTrust, 1)
				s.RewardExpAndSp(p, 39571, 2500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
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
	case dryad, dryadElder:
		if st.Cond() == 2 && !p.HasItem(seedOfVerdure) && s.Rnd(100) < 33 {
			s.Spawner().AddSpawn(acteaOfVerdantWilds, e.NPC, true, guardianDespawn)
			s.PlaySound(p, script.SoundBeforeBattle)
		}
	case lirein, lireinElder:
		if st.Cond() == 2 && !p.HasItem(breathOfWinds) && s.Rnd(100) < 33 {
			s.Spawner().AddSpawn(luellOfZephyrWinds, e.NPC, true, guardianDespawn)
			s.PlaySound(p, script.SoundBeforeBattle)
		}
	case acteaOfVerdantWilds:
		if st.Cond() == 2 && !p.HasItem(seedOfVerdure) {
			s.GiveItems(p, seedOfVerdure, 1)
			if p.HasItem(breathOfWinds) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case luellOfZephyrWinds:
		if st.Cond() == 2 && !p.HasItem(breathOfWinds) {
			s.GiveItems(p, breathOfWinds, 1)
			if p.HasItem(seedOfVerdure) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case marshStakato, marshStakatoWorker, marshStakatoSoldier, marshStakatoDrone:
		if st.Cond() == 6 && !p.HasItem(stakatoIchor) && s.DropItemsAlways(p, stakatoFluids, 1, 10) {
			s.TakeItems(p, stakatoFluids, -1)
			s.GiveItems(p, stakatoIchor, 1)
			if p.HasItems(basilikPlasma, honeyDew) {
				st.SetCond(7)
			}
		}
	case antRecruit, antPatrol, antGuard, antSoldier, antWarriorCaptain:
		if st.Cond() == 6 && !p.HasItem(honeyDew) && s.DropItemsAlways(p, giantAphid, 1, 10) {
			s.TakeItems(p, giantAphid, -1)
			s.GiveItems(p, honeyDew, 1)
			if p.HasItems(basilikPlasma, stakatoIchor) {
				st.SetCond(7)
			}
		}
	case guardianBasilik:
		if st.Cond() == 6 && !p.HasItem(basilikPlasma) && s.DropItemsAlways(p, bloodGuardianBasilik, 1, 10) {
			s.TakeItems(p, bloodGuardianBasilik, -1)
			s.GiveItems(p, basilikPlasma, 1)
			if p.HasItems(honeyDew, stakatoIchor) {
				st.SetCond(7)
			}
		}
	case windsus:
		if st.Cond() == 14 && s.DropItems(p, parasiteOfLota, 1, 10, 500000) {
			st.SetCond(15)
		}
	case porta:
		if st.Cond() == 19 && s.DropItemsAlways(p, heartstoneOfPorta, 1, 10) {
			st.SetCond(20)
		}
	}
}
