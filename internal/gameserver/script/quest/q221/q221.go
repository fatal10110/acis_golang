// Package q221 is the quest Testimony of Prosperity, a second class-change
// testimony for dwarves: Parman sends the traveller to gather four goods
// (a seed, an account book, a recipe and a wafer, the book by way of the
// iron gate's collection round), then to Nikola and the Box of Titan for a
// key made of stakato, toad and spider trophies, and back to Parman for the
// Mark of Prosperity.
package q221

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q221_TestimonyOfProsperity"

// Items the quest does not list as its own.
const (
	adena          = 57
	animalSkin     = 1867
	recipeTitanKey = 3023
	keyOfTitan     = 3030
)

// Items.
const (
	ringOfTestimony1       = 3239
	ringOfTestimony2       = 3240
	oldAccountBook         = 3241
	blessedSeed            = 3242
	emilyRecipe            = 3243
	lilithElvenWafer       = 3244
	maphrTabletFragment    = 3245
	collectionLicense      = 3246
	lockirinNotice1        = 3247
	lockirinNotice2        = 3248
	lockirinNotice3        = 3249
	lockirinNotice4        = 3250
	lockirinNotice5        = 3251
	contributionOfShari    = 3252
	contributionOfMion     = 3253
	contributionOfMaryse   = 3254
	maryseRequest          = 3255
	contributionOfToma     = 3256
	receiptOfBolter        = 3257
	receiptOfContribution1 = 3258
	receiptOfContribution2 = 3259
	receiptOfContribution3 = 3260
	receiptOfContribution4 = 3261
	receiptOfContribution5 = 3262
	procurationOfTorocco   = 3263
	brightList             = 3264
	mandragoraPetal        = 3265
	crimsonMoss            = 3266
	mandragoraBouquet      = 3267
	parmanInstructions     = 3268
	parmanLetter           = 3269
	clayDough              = 3270
	patternOfKeyhole       = 3271
	nikolasList            = 3272
	stakatoShell           = 3273
	toadLordSac            = 3274
	spiderThorn            = 3275
	crystalBrooch          = 3428
)

// Rewards.
const markOfProsperity = 3238

// NPCs.
const (
	wilford         = 30005
	parman          = 30104
	lilith          = 30368
	bright          = 30466
	shari           = 30517
	mion            = 30519
	lockirin        = 30531
	spiron          = 30532
	balanki         = 30533
	keef            = 30534
	filaur          = 30535
	arin            = 30536
	maryseRedbonnet = 30553
	bolter          = 30554
	torocco         = 30555
	toma            = 30556
	piotur          = 30597
	emily           = 30620
	nikola          = 30621
	boxOfTitan      = 30622
)

// Monsters.
const (
	mandragoraSprout1   = 20223
	mandragoraSprout2   = 20154
	mandragoraSapling   = 20155
	mandragoraBlossom   = 20156
	marshStakato        = 20157
	giantCrimsonAnt     = 20228
	marshStakatoWorker  = 20230
	toadLord            = 20231
	marshStakatoSoldier = 20232
	marshSpider         = 20233
	marshStakatoDrone   = 20234
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Testimony Of Prosperity",
		QuestID: 221,
		Items: []int32{
			ringOfTestimony1, ringOfTestimony2, oldAccountBook, blessedSeed, emilyRecipe, lilithElvenWafer,
			maphrTabletFragment, collectionLicense, lockirinNotice1, lockirinNotice2, lockirinNotice3,
			lockirinNotice4, lockirinNotice5, contributionOfShari, contributionOfMion, contributionOfMaryse,
			maryseRequest, contributionOfToma, receiptOfBolter, receiptOfContribution1, receiptOfContribution2,
			receiptOfContribution3, receiptOfContribution4, receiptOfContribution5, procurationOfTorocco,
			brightList, mandragoraPetal, crimsonMoss, mandragoraBouquet, parmanInstructions, parmanLetter,
			clayDough, patternOfKeyhole, nikolasList, stakatoShell, toadLordSac, spiderThorn, crystalBrooch,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {parman},
			script.EventTalked: {
				wilford, parman, lilith, bright, shari, mion, lockirin, spiron, balanki, keef, filaur, arin,
				maryseRedbonnet, bolter, torocco, toma, piotur, emily, nikola, boxOfTitan,
			},
			script.EventMyDying: {
				mandragoraSprout1, mandragoraSapling, mandragoraBlossom, marshStakato, mandragoraSprout2,
				giantCrimsonAnt, marshStakatoWorker, toadLord, marshStakatoSoldier, marshSpider,
				marshStakatoDrone,
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
	case strings.EqualFold(e.Name, "30104-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ringOfTestimony1, 1)
		if secondclass.GiveDimensionalDiamonds37(s, p) {
			htmltext = "30104-04e.htm"
		}
	case strings.EqualFold(e.Name, "30104-07.htm"):
		s.TakeItems(p, blessedSeed, 1)
		s.TakeItems(p, emilyRecipe, 1)
		s.TakeItems(p, lilithElvenWafer, 1)
		s.TakeItems(p, oldAccountBook, 1)
		s.TakeItems(p, ringOfTestimony1, 1)
		s.PlaySound(p, script.SoundMiddle)
		if p.Level() < 38 {
			st.SetCond(3)
			s.GiveItems(p, parmanInstructions, 1)
		} else {
			htmltext = "30104-08.htm"
			st.SetCond(4)
			s.GiveItems(p, parmanLetter, 1)
			s.GiveItems(p, ringOfTestimony2, 1)
		}
	case strings.EqualFold(e.Name, "30531-02.htm") && p.HasItem(collectionLicense):
		htmltext = "30531-04.htm"
	case strings.EqualFold(e.Name, "30531-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, collectionLicense, 1)
		s.GiveItems(p, lockirinNotice1, 1)
		s.GiveItems(p, lockirinNotice2, 1)
		s.GiveItems(p, lockirinNotice3, 1)
		s.GiveItems(p, lockirinNotice4, 1)
		s.GiveItems(p, lockirinNotice5, 1)
	case strings.EqualFold(e.Name, "30534-03a.htm") && p.ItemCount(adena) >= 5000:
		htmltext = "30534-03b.htm"
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, adena, 5000)
		s.TakeItems(p, procurationOfTorocco, 1)
		s.GiveItems(p, receiptOfContribution3, 1)
	case strings.EqualFold(e.Name, "30005-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, crystalBrooch, 1)
	case strings.EqualFold(e.Name, "30466-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, brightList, 1)
	case strings.EqualFold(e.Name, "30555-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, procurationOfTorocco, 1)
	case strings.EqualFold(e.Name, "30368-03.htm"):
		s.TakeItems(p, crystalBrooch, 1)
		s.GiveItems(p, lilithElvenWafer, 1)
		if p.HasItems(blessedSeed, oldAccountBook, emilyRecipe) {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "30597-02.htm"):
		s.GiveItems(p, blessedSeed, 1)
		if p.HasItems(oldAccountBook, emilyRecipe, lilithElvenWafer) {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "30620-03.htm"):
		s.TakeItems(p, mandragoraBouquet, 1)
		s.GiveItems(p, emilyRecipe, 1)
		if p.HasItems(blessedSeed, oldAccountBook, lilithElvenWafer) {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "30621-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, clayDough, 1)
	case strings.EqualFold(e.Name, "30622-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, clayDough, 1)
		s.GiveItems(p, patternOfKeyhole, 1)
	case strings.EqualFold(e.Name, "30622-04.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, keyOfTitan, 1)
		s.TakeItems(p, nikolasList, 1)
		s.TakeItems(p, recipeTitanKey, 1)
		s.TakeItems(p, stakatoShell, 20)
		s.TakeItems(p, spiderThorn, 10)
		s.TakeItems(p, toadLordSac, 10)
		s.GiveItems(p, maphrTabletFragment, 1)
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
		case p.Race() != player.RaceDwarf:
			htmltext = "30104-01.htm"
		case p.Level() < 37:
			htmltext = "30104-02.htm"
		case p.ClassLevel() != 1:
			htmltext = "30104-01a.htm"
		default:
			htmltext = "30104-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e, st, e.NPC.NpcID(), st.Cond())
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the dialog of a traveller whose quest is running.
func talkStarted(s *script.Script, e script.Talk, st *script.QuestState, npcID, cond int32) string {
	p := e.Player
	htmltext := script.NoQuestMsg()
	switch npcID {
	case parman:
		switch {
		case cond == 1:
			htmltext = "30104-05.htm"
		case cond == 2:
			htmltext = "30104-06.htm"
		case cond == 3:
			if p.Level() < 38 {
				htmltext = "30104-09.htm"
			} else {
				htmltext = "30104-10.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, parmanInstructions, 1)
				s.GiveItems(p, parmanLetter, 1)
				s.GiveItems(p, ringOfTestimony2, 1)
			}
		case cond > 3 && cond < 7:
			htmltext = "30104-11.htm"
		case cond == 7 || cond == 8:
			htmltext = "30104-12.htm"
		case cond == 9:
			htmltext = "30104-13.htm"
			s.TakeItems(p, maphrTabletFragment, 1)
			s.TakeItems(p, ringOfTestimony2, 1)
			s.GiveItems(p, markOfProsperity, 1)
			s.RewardExpAndSp(p, 12969, 1000)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case lockirin:
		switch {
		case cond == 1 || cond == 2:
			if p.HasItem(collectionLicense) {
				if p.HasItems(receiptOfContribution1, receiptOfContribution2, receiptOfContribution3, receiptOfContribution4, receiptOfContribution5) {
					htmltext = "30531-05.htm"
					s.TakeItems(p, collectionLicense, 1)
					s.TakeItems(p, receiptOfContribution1, 1)
					s.TakeItems(p, receiptOfContribution2, 1)
					s.TakeItems(p, receiptOfContribution3, 1)
					s.TakeItems(p, receiptOfContribution4, 1)
					s.TakeItems(p, receiptOfContribution5, 1)
					s.GiveItems(p, oldAccountBook, 1)
					if p.HasItems(blessedSeed, emilyRecipe, lilithElvenWafer) {
						st.SetCond(2)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				} else {
					htmltext = "30531-04.htm"
				}
			} else if p.HasItem(oldAccountBook) {
				htmltext = "30531-06.htm"
			} else {
				htmltext = "30531-01.htm"
			}
		case cond >= 4:
			htmltext = "30531-07.htm"
		}
	case spiron:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(lockirinNotice1):
				htmltext = "30532-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, lockirinNotice1, 1)
			case p.HasItem(contributionOfShari):
				htmltext = "30532-03.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, contributionOfShari, 1)
				s.GiveItems(p, receiptOfContribution1, 1)
			case p.HasItem(receiptOfContribution1):
				htmltext = "30532-04.htm"
			default:
				htmltext = "30532-02.htm"
			}
		}
	case balanki:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(lockirinNotice2):
				htmltext = "30533-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, lockirinNotice2, 1)
			case p.HasItems(contributionOfMaryse, contributionOfMion):
				htmltext = "30533-03.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, contributionOfMaryse, 1)
				s.TakeItems(p, contributionOfMion, 1)
				s.GiveItems(p, receiptOfContribution2, 1)
			case p.HasItem(receiptOfContribution2):
				htmltext = "30533-04.htm"
			default:
				htmltext = "30533-02.htm"
			}
		}
	case keef:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(lockirinNotice3):
				htmltext = "30534-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, lockirinNotice3, 1)
			case p.HasItem(procurationOfTorocco):
				htmltext = "30534-03.htm"
			case p.HasItem(receiptOfContribution3):
				htmltext = "30534-04.htm"
			default:
				htmltext = "30534-02.htm"
			}
		}
	case filaur:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(lockirinNotice4):
				htmltext = "30535-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, lockirinNotice4, 1)
			case p.HasItem(receiptOfBolter):
				htmltext = "30535-03.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, receiptOfBolter, 1)
				s.GiveItems(p, receiptOfContribution4, 1)
			case p.HasItem(receiptOfContribution4):
				htmltext = "30535-04.htm"
			default:
				htmltext = "30535-02.htm"
			}
		}
	case arin:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(lockirinNotice5):
				htmltext = "30536-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, lockirinNotice5, 1)
			case p.HasItem(contributionOfToma):
				htmltext = "30536-03.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, contributionOfToma, 1)
				s.GiveItems(p, receiptOfContribution5, 1)
			case p.HasItem(receiptOfContribution5):
				htmltext = "30536-04.htm"
			default:
				htmltext = "30536-02.htm"
			}
		}
	case shari:
		if cond == 1 && p.HasItem(collectionLicense) {
			if p.HasItem(contributionOfShari) {
				htmltext = "30517-02.htm"
			} else if !p.HasAtLeastOneItem(lockirinNotice1, receiptOfContribution1) {
				htmltext = "30517-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, contributionOfShari, 1)
			}
		}
	case mion:
		if cond == 1 && p.HasItem(collectionLicense) {
			if p.HasItem(contributionOfMion) {
				htmltext = "30519-02.htm"
			} else if !p.HasAtLeastOneItem(lockirinNotice2, receiptOfContribution2) {
				htmltext = "30519-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, contributionOfMion, 1)
			}
		}
	case maryseRedbonnet:
		if cond == 1 && p.HasItem(collectionLicense) {
			switch {
			case p.HasItem(maryseRequest):
				if p.ItemCount(animalSkin) < 100 {
					htmltext = "30553-02.htm"
				} else {
					htmltext = "30553-03.htm"
					s.PlaySound(p, script.SoundItemGet)
					s.TakeItems(p, animalSkin, 100)
					s.TakeItems(p, maryseRequest, 1)
					s.GiveItems(p, contributionOfMaryse, 1)
				}
			case p.HasItem(contributionOfMaryse):
				htmltext = "30553-04.htm"
			case !p.HasAtLeastOneItem(lockirinNotice2, receiptOfContribution2):
				htmltext = "30553-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, maryseRequest, 1)
			}
		}
	case torocco:
		if cond == 1 && p.HasItem(collectionLicense) {
			if p.HasItem(procurationOfTorocco) {
				htmltext = "30555-03.htm"
			} else if !p.HasAtLeastOneItem(lockirinNotice3, receiptOfContribution3) {
				htmltext = "30555-01.htm"
			}
		}
	case bolter:
		if cond == 1 && p.HasItem(collectionLicense) {
			if p.HasItem(receiptOfBolter) {
				htmltext = "30554-02.htm"
			} else if !p.HasAtLeastOneItem(lockirinNotice4, receiptOfContribution4) {
				htmltext = "30554-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, receiptOfBolter, 1)
			}
		}
	case toma:
		if cond == 1 && p.HasItem(collectionLicense) {
			if p.HasItem(contributionOfToma) {
				htmltext = "30556-02.htm"
			} else if !p.HasAtLeastOneItem(lockirinNotice5, receiptOfContribution5) {
				htmltext = "30556-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, contributionOfToma, 1)
			}
		}
	case piotur:
		switch {
		case cond == 1 || cond == 2:
			if p.HasItem(blessedSeed) {
				htmltext = "30597-03.htm"
			} else {
				htmltext = "30597-01.htm"
			}
		case cond >= 4:
			htmltext = "30597-04.htm"
		}
	case wilford:
		switch {
		case cond == 1 || cond == 2:
			switch {
			case p.HasItem(lilithElvenWafer):
				htmltext = "30005-06.htm"
			case p.HasItem(crystalBrooch):
				htmltext = "30005-05.htm"
			default:
				htmltext = "30005-01.htm"
			}
		case cond >= 4:
			htmltext = "30005-07.htm"
		}
	case lilith:
		switch {
		case cond == 1 || cond == 2:
			if p.HasItem(crystalBrooch) {
				htmltext = "30368-01.htm"
			} else if p.HasItem(lilithElvenWafer) {
				htmltext = "30368-04.htm"
			}
		case cond >= 4:
			htmltext = "30368-05.htm"
		}
	case bright:
		switch {
		case cond == 1 || cond == 2:
			switch {
			case p.HasItem(emilyRecipe):
				htmltext = "30466-07.htm"
			case p.HasItem(mandragoraBouquet):
				htmltext = "30466-06.htm"
			case p.HasItem(brightList):
				if p.ItemCount(crimsonMoss)+p.ItemCount(mandragoraPetal) < 30 {
					htmltext = "30466-04.htm"
				} else {
					htmltext = "30466-05.htm"
					s.PlaySound(p, script.SoundItemGet)
					s.TakeItems(p, brightList, 1)
					s.TakeItems(p, crimsonMoss, 10)
					s.TakeItems(p, mandragoraPetal, 20)
					s.GiveItems(p, mandragoraBouquet, 1)
				}
			default:
				htmltext = "30466-01.htm"
			}
		case cond >= 4:
			htmltext = "30466-08.htm"
		}
	case emily:
		switch {
		case cond == 1 || cond == 2:
			if p.HasItem(emilyRecipe) {
				htmltext = "30620-04.htm"
			} else if p.HasItem(mandragoraBouquet) {
				htmltext = "30620-01.htm"
			}
		case cond >= 4:
			htmltext = "30620-05.htm"
		}
	case nikola:
		switch {
		case cond == 4:
			htmltext = "30621-01.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, parmanLetter, 1)
		case cond == 5:
			htmltext = "30621-05.htm"
		case cond == 6:
			htmltext = "30621-06.htm"
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, patternOfKeyhole, 1)
			s.GiveItems(p, nikolasList, 1)
			s.GiveItems(p, recipeTitanKey, 1)
		case cond == 7 || cond == 8:
			if p.HasItem(keyOfTitan) {
				htmltext = "30621-08.htm"
			} else {
				htmltext = "30621-07.htm"
			}
		case cond == 9:
			htmltext = "30621-09.htm"
		}
	case boxOfTitan:
		switch {
		case cond == 5:
			htmltext = "30622-01.htm"
		case cond == 8 && p.HasItem(keyOfTitan):
			htmltext = "30622-03.htm"
		default:
			htmltext = "30622-05.htm"
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
	case mandragoraSprout1:
		if p.HasItem(brightList) {
			s.DropItems(p, mandragoraPetal, 1, 20, 300000)
		}
	case mandragoraSprout2:
		if p.HasItem(brightList) {
			s.DropItems(p, mandragoraPetal, 1, 20, 600000)
		}
	case mandragoraSapling:
		if p.HasItem(brightList) {
			s.DropItems(p, mandragoraPetal, 1, 20, 800000)
		}
	case mandragoraBlossom:
		if p.HasItem(brightList) {
			s.DropItemsAlways(p, mandragoraPetal, 1, 20)
		}
	case giantCrimsonAnt:
		if p.HasItem(brightList) {
			s.DropItemsAlways(p, crimsonMoss, 1, 10)
		}
	case marshStakato:
		dropShell(s, p, st, 200000)
	case marshStakatoWorker:
		dropShell(s, p, st, 300000)
	case marshStakatoSoldier:
		dropShell(s, p, st, 500000)
	case marshStakatoDrone:
		dropShell(s, p, st, 600000)
	case toadLord:
		if st.Cond() == 7 && s.DropItems(p, toadLordSac, 1, 10, 200000) &&
			p.ItemCount(stakatoShell)+p.ItemCount(spiderThorn) == 30 {
			st.SetCond(8)
		}
	case marshSpider:
		if st.Cond() == 7 && s.DropItems(p, spiderThorn, 1, 10, 200000) &&
			p.ItemCount(stakatoShell)+p.ItemCount(toadLordSac) == 30 {
			st.SetCond(8)
		}
	}
}

// dropShell is a stakato's kill at cond 7: the shell drops at the given
// chance, and the last of the three trophies moves the quest on.
func dropShell(s *script.Script, p *script.Player, st *script.QuestState, chance int32) {
	if st.Cond() == 7 && s.DropItems(p, stakatoShell, 1, 20, chance) &&
		p.ItemCount(toadLordSac)+p.ItemCount(spiderThorn) == 20 {
		st.SetCond(8)
	}
}
