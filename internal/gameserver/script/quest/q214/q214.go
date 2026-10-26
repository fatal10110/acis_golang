// Package q214 is the quest Trial Of The Scholar, a second class-change test
// for wizards: Mirien sends the traveller through the scholars of Talking
// Island and the Elven Fortress, the high priest Sylvain, the grand magister
// Jurek and the sage Cronos, collecting a sigil from each, for the Mark of
// Scholar.
package q214

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q214_TrialOfTheScholar"

// Items.
const (
	mirienSigil1            = 2675
	mirienSigil2            = 2676
	mirienSigil3            = 2677
	mirienInstruction       = 2678
	mariaLetter1            = 2679
	mariaLetter2            = 2680
	lucasLetter             = 2681
	lucillaHandbag          = 2682
	cretaLetter1            = 2683
	cretaPainting1          = 2684
	cretaPainting2          = 2685
	cretaPainting3          = 2686
	brownScrollScrap        = 2687
	crystalOfPurity1        = 2688
	highPriestSigil         = 2689
	grandMagisterSigil      = 2690
	cronosSigil             = 2691
	sylvainLetter           = 2692
	symbolOfSylvain         = 2693
	jurekList               = 2694
	monsterEyeDestroyerSkin = 2695
	shamanNecklace          = 2696
	shackleScalp            = 2697
	symbolOfJurek           = 2698
	cronosLetter            = 2699
	dieterKey               = 2700
	cretaLetter2            = 2701
	dieterLetter            = 2702
	dieterDiary             = 2703
	rautLetterEnvelope      = 2704
	triffRing               = 2705
	scriptureChapter1       = 2706
	scriptureChapter2       = 2707
	scriptureChapter3       = 2708
	scriptureChapter4       = 2709
	valkonRequest           = 2710
	poitanNotes             = 2711
	strongLiquor            = 2713
	crystalOfPurity2        = 2714
	casianList              = 2715
	ghoulSkin               = 2716
	medusaBlood             = 2717
	fetteredSoulIchor       = 2718
	enchantedGargoyleNail   = 2719
	symbolOfCronos          = 2720
)

// Rewards.
const markOfScholar = 2674

// NPCs.
const (
	sylvain = 30070
	lucas   = 30071
	valkon  = 30103
	dieter  = 30111
	jurek   = 30115
	edroc   = 30230
	raut    = 30316
	poitan  = 30458
	mirien  = 30461
	maria   = 30608
	creta   = 30609
	cronos  = 30610
	triff   = 30611
	casian  = 30612
)

// Monsters.
const (
	monsterEyeDestroyer  = 20068
	medusa               = 20158
	ghoul                = 20201
	shackle1             = 20235
	shackle2             = 20279
	brekaOrcShaman       = 20269
	fetteredSoul         = 20552
	grandis              = 20554
	enchantedGargoyle    = 20567
	letoLizardmanWarrior = 20580
)

// The classes the trial is open to.
const (
	classHumanWizard = 11
	classElvenWizard = 26
	classDarkWizard  = 39
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial Of The Scholar",
		QuestID: 214,
		Items: []int32{
			mirienSigil1, mirienSigil2, mirienSigil3, mirienInstruction, mariaLetter1, mariaLetter2, lucasLetter,
			lucillaHandbag, cretaLetter1, cretaPainting1, cretaPainting2, cretaPainting3, brownScrollScrap,
			crystalOfPurity1, highPriestSigil, grandMagisterSigil, cronosSigil, sylvainLetter, symbolOfSylvain,
			jurekList, monsterEyeDestroyerSkin, shamanNecklace, shackleScalp, symbolOfJurek, cronosLetter, dieterKey,
			cretaLetter2, dieterLetter, dieterDiary, rautLetterEnvelope, triffRing, scriptureChapter1,
			scriptureChapter2, scriptureChapter3, scriptureChapter4, valkonRequest, poitanNotes, strongLiquor,
			crystalOfPurity2, casianList, ghoulSkin, medusaBlood, fetteredSoulIchor, enchantedGargoyleNail,
			symbolOfCronos,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {mirien},
			script.EventTalked: {
				mirien, sylvain, lucas, valkon, dieter, jurek, edroc, raut, poitan, maria, creta, cronos, triff, casian,
			},
			script.EventMyDying: {
				monsterEyeDestroyer, medusa, ghoul, shackle1, shackle2, brekaOrcShaman, fetteredSoul, grandis,
				enchantedGargoyle, letoLizardmanWarrior,
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
	// Mirien.
	case strings.EqualFold(e.Name, "30461-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, mirienSigil1, 1)
		if secondclass.GiveDimensionalDiamonds35(s, p) {
			htmltext = "30461-04a.htm"
		}
	case strings.EqualFold(e.Name, "30461-09.htm"):
		if p.Level() < 36 {
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, mirienInstruction, 1)
		} else {
			htmltext = "30461-10.htm"
			st.SetCond(19)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, mirienSigil2, 1)
			s.TakeItems(p, symbolOfJurek, 1)
			s.GiveItems(p, mirienSigil3, 1)
		}
	// Sylvain.
	case strings.EqualFold(e.Name, "30070-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, highPriestSigil, 1)
		s.GiveItems(p, sylvainLetter, 1)
	// Maria.
	case strings.EqualFold(e.Name, "30608-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sylvainLetter, 1)
		s.GiveItems(p, mariaLetter1, 1)
	case strings.EqualFold(e.Name, "30608-08.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cretaLetter1, 1)
		s.GiveItems(p, lucillaHandbag, 1)
	case strings.EqualFold(e.Name, "30608-14.htm"):
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, brownScrollScrap, -1)
		s.TakeItems(p, cretaPainting3, 1)
		s.GiveItems(p, crystalOfPurity1, 1)
	// Jurek.
	case strings.EqualFold(e.Name, "30115-03.htm"):
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, grandMagisterSigil, 1)
		s.GiveItems(p, jurekList, 1)
	// Lucas.
	case strings.EqualFold(e.Name, "30071-04.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cretaPainting2, 1)
		s.GiveItems(p, cretaPainting3, 1)
	// Creta.
	case strings.EqualFold(e.Name, "30609-05.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mariaLetter2, 1)
		s.GiveItems(p, cretaLetter1, 1)
	case strings.EqualFold(e.Name, "30609-09.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, lucillaHandbag, 1)
		s.GiveItems(p, cretaPainting1, 1)
	case strings.EqualFold(e.Name, "30609-14.htm"):
		st.SetCond(22)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, dieterKey, 1)
		s.GiveItems(p, cretaLetter2, 1)
	// Cronos.
	case strings.EqualFold(e.Name, "30610-10.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, cronosLetter, 1)
		s.GiveItems(p, cronosSigil, 1)
	case strings.EqualFold(e.Name, "30610-14.htm"):
		st.SetCond(31)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cronosSigil, 1)
		s.TakeItems(p, dieterDiary, 1)
		s.TakeItems(p, scriptureChapter1, 1)
		s.TakeItems(p, scriptureChapter2, 1)
		s.TakeItems(p, scriptureChapter3, 1)
		s.TakeItems(p, scriptureChapter4, 1)
		s.TakeItems(p, triffRing, 1)
		s.GiveItems(p, symbolOfCronos, 1)
	// Dieter.
	case strings.EqualFold(e.Name, "30111-05.htm"):
		st.SetCond(21)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cronosLetter, 1)
		s.GiveItems(p, dieterKey, 1)
	case strings.EqualFold(e.Name, "30111-09.htm"):
		st.SetCond(23)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cretaLetter2, 1)
		s.GiveItems(p, dieterDiary, 1)
		s.GiveItems(p, dieterLetter, 1)
	// Edroc.
	case strings.EqualFold(e.Name, "30230-02.htm"):
		st.SetCond(24)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, dieterLetter, 1)
		s.GiveItems(p, rautLetterEnvelope, 1)
	// Raut.
	case strings.EqualFold(e.Name, "30316-02.htm"):
		st.SetCond(25)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rautLetterEnvelope, 1)
		s.GiveItems(p, scriptureChapter1, 1)
		s.GiveItems(p, strongLiquor, 1)
	// Triff.
	case strings.EqualFold(e.Name, "30611-04.htm"):
		st.SetCond(26)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, strongLiquor, 1)
		s.GiveItems(p, triffRing, 1)
	// Valkon.
	case strings.EqualFold(e.Name, "30103-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, valkonRequest, 1)
	// Casian.
	case strings.EqualFold(e.Name, "30612-04.htm"):
		st.SetCond(28)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, casianList, 1)
	case strings.EqualFold(e.Name, "30612-07.htm"):
		st.SetCond(30)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, casianList, 1)
		s.TakeItems(p, enchantedGargoyleNail, -1)
		s.TakeItems(p, fetteredSoulIchor, -1)
		s.TakeItems(p, ghoulSkin, -1)
		s.TakeItems(p, medusaBlood, -1)
		s.TakeItems(p, poitanNotes, 1)
		s.GiveItems(p, scriptureChapter4, 1)
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
			htmltext = "30461-01.htm"
		case p.Level() < 35:
			htmltext = "30461-02.htm"
		default:
			htmltext = "30461-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started trial, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case mirien:
		switch {
		case cond < 14:
			htmltext = "30461-05.htm"
		case cond == 14:
			htmltext = "30461-06.htm"
			st.SetCond(15)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, mirienSigil1, 1)
			s.TakeItems(p, symbolOfSylvain, 1)
			s.GiveItems(p, mirienSigil2, 1)
		case cond > 14 && cond < 18:
			htmltext = "30461-07.htm"
		case cond == 18:
			switch {
			case !p.HasItem(mirienInstruction):
				htmltext = "30461-08.htm"
			case p.Level() < 36:
				htmltext = "30461-11.htm"
			default:
				htmltext = "30461-12.htm"
				st.SetCond(19)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, mirienInstruction, 1)
				s.TakeItems(p, mirienSigil2, 1)
				s.TakeItems(p, symbolOfJurek, 1)
				s.GiveItems(p, mirienSigil3, 1)
			}
		case cond > 18 && cond < 31:
			htmltext = "30461-13.htm"
		case cond == 31:
			htmltext = "30461-14.htm"
			s.TakeItems(p, mirienSigil3, 1)
			s.TakeItems(p, symbolOfCronos, 1)
			s.GiveItems(p, markOfScholar, 1)
			s.RewardExpAndSp(p, 80265, 30000)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case sylvain:
		switch {
		case cond == 1:
			htmltext = "30070-01.htm"
		case cond < 13:
			htmltext = "30070-03.htm"
		case cond == 13:
			htmltext = "30070-04.htm"
			st.SetCond(14)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, crystalOfPurity1, 1)
			s.TakeItems(p, highPriestSigil, 1)
			s.GiveItems(p, symbolOfSylvain, 1)
		case cond == 14:
			htmltext = "30070-05.htm"
		case cond > 14:
			htmltext = "30070-06.htm"
		}
	case maria:
		htmltext = talkMaria(s, p, st, cond)
	case jurek:
		switch {
		case cond == 15:
			htmltext = "30115-01.htm"
		case cond == 16:
			htmltext = "30115-04.htm"
		case cond == 17:
			htmltext = "30115-05.htm"
			st.SetCond(18)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, grandMagisterSigil, 1)
			s.TakeItems(p, jurekList, 1)
			s.TakeItems(p, monsterEyeDestroyerSkin, -1)
			s.TakeItems(p, shackleScalp, -1)
			s.TakeItems(p, shamanNecklace, -1)
			s.GiveItems(p, symbolOfJurek, 1)
		case cond == 18:
			htmltext = "30115-06.htm"
		case cond > 18:
			htmltext = "30115-07.htm"
		}
	case lucas:
		switch {
		case cond == 3:
			htmltext = "30071-01.htm"
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, mariaLetter1, 1)
			s.GiveItems(p, lucasLetter, 1)
		case cond > 3 && cond < 9:
			htmltext = "30071-02.htm"
		case cond == 9:
			htmltext = "30071-03.htm"
		case cond == 10 || cond == 11:
			htmltext = "30071-05.htm"
		case cond == 12:
			htmltext = "30071-06.htm"
		case cond > 12:
			htmltext = "30071-07.htm"
		}
	case creta:
		switch {
		case cond == 5:
			htmltext = "30609-01.htm"
		case cond == 6:
			htmltext = "30609-06.htm"
		case cond == 7:
			htmltext = "30609-07.htm"
		case cond > 7 && cond < 13:
			htmltext = "30609-10.htm"
		case cond >= 13 && cond < 19:
			htmltext = "30609-11.htm"
		case cond == 21:
			htmltext = "30609-12.htm"
		case cond > 21:
			htmltext = "30609-15.htm"
		}
	case cronos:
		switch {
		case cond == 19:
			htmltext = "30610-01.htm"
		case cond > 19 && cond < 30:
			htmltext = "30610-11.htm"
		case cond == 30:
			htmltext = "30610-12.htm"
		case cond == 31:
			htmltext = "30610-15.htm"
		}
	case dieter:
		switch {
		case cond == 20:
			htmltext = "30111-01.htm"
		case cond == 21:
			htmltext = "30111-06.htm"
		case cond == 22:
			htmltext = "30111-07.htm"
		case cond == 23:
			htmltext = "30111-10.htm"
		case cond == 24:
			htmltext = "30111-11.htm"
		case cond > 24 && cond < 31:
			if !p.HasItems(scriptureChapter1, scriptureChapter2, scriptureChapter3, scriptureChapter4) {
				htmltext = "30111-12.htm"
			} else {
				htmltext = "30111-13.htm"
			}
		case cond == 31:
			htmltext = "30111-15.htm"
		}
	case edroc:
		switch {
		case cond == 23:
			htmltext = "30230-01.htm"
		case cond == 24:
			htmltext = "30230-03.htm"
		case cond > 24:
			htmltext = "30230-04.htm"
		}
	case raut:
		switch {
		case cond == 24:
			htmltext = "30316-01.htm"
		case cond == 25:
			htmltext = "30316-04.htm"
		case cond > 25:
			htmltext = "30316-05.htm"
		}
	case triff:
		switch {
		case cond == 25:
			htmltext = "30611-01.htm"
		case cond > 25:
			htmltext = "30611-05.htm"
		}
	case valkon:
		htmltext = talkValkon(s, p, htmltext)
	case poitan:
		switch {
		case cond == 26 || cond == 27:
			if !p.HasItem(poitanNotes) {
				htmltext = "30458-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, poitanNotes, 1)
			} else {
				htmltext = "30458-02.htm"
			}
		case cond == 28 || cond == 29:
			htmltext = "30458-03.htm"
		case cond == 30:
			htmltext = "30458-04.htm"
		}
	case casian:
		switch {
		case (cond == 26 || cond == 27) && p.HasItem(poitanNotes):
			if p.HasItems(scriptureChapter1, scriptureChapter2, scriptureChapter3) {
				htmltext = "30612-02.htm"
			} else {
				htmltext = "30612-01.htm"
				if cond == 26 {
					st.SetCond(27)
					s.PlaySound(p, script.SoundMiddle)
				}
			}
		case cond == 28:
			htmltext = "30612-05.htm"
		case cond == 29:
			htmltext = "30612-06.htm"
		case cond == 30:
			htmltext = "30612-08.htm"
		}
	}
	return htmltext
}

// talkMaria is the page Maria gives a started trial at cond, with the step
// it takes. Once the trial is past its eighteenth step she also trades
// Valkon's request for the second crystal of purity.
func talkMaria(s *script.Script, p *script.Player, st *script.QuestState, cond int32) string {
	htmltext := script.NoQuestMsg()
	switch {
	case cond == 2:
		htmltext = "30608-01.htm"
	case cond == 3:
		htmltext = "30608-03.htm"
	case cond == 4:
		htmltext = "30608-04.htm"
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, lucasLetter, 1)
		s.GiveItems(p, mariaLetter2, 1)
	case cond == 5:
		htmltext = "30608-05.htm"
	case cond == 6:
		htmltext = "30608-06.htm"
	case cond == 7:
		htmltext = "30608-09.htm"
	case cond == 8:
		htmltext = "30608-10.htm"
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cretaPainting1, 1)
		s.GiveItems(p, cretaPainting2, 1)
	case cond == 9:
		htmltext = "30608-11.htm"
	case cond == 10:
		htmltext = "30608-12.htm"
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
	case cond == 11:
		htmltext = "30608-12.htm"
	case cond == 12:
		htmltext = "30608-13.htm"
	case cond == 13:
		htmltext = "30608-15.htm"
	case p.HasAtLeastOneItem(symbolOfSylvain, mirienSigil2):
		htmltext = "30608-16.htm"
	case cond > 18:
		if !p.HasItem(valkonRequest) {
			htmltext = "30608-17.htm"
		} else {
			htmltext = "30608-18.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, valkonRequest, 1)
			s.GiveItems(p, crystalOfPurity2, 1)
		}
	}
	return htmltext
}

// talkValkon is the page Valkon gives a trial that holds Triff's ring, with
// the step it takes; without the ring he has nothing to say, whatever the
// cond.
func talkValkon(s *script.Script, p *script.Player, htmltext string) string {
	switch {
	case !p.HasItem(triffRing):
	case p.HasItem(scriptureChapter2):
		htmltext = "30103-07.htm"
	case p.HasItem(valkonRequest):
		htmltext = "30103-05.htm"
	case !p.HasItem(crystalOfPurity2):
		htmltext = "30103-01.htm"
	default:
		htmltext = "30103-06.htm"
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, crystalOfPurity2, 1)
		s.GiveItems(p, scriptureChapter2, 1)
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
	case letoLizardmanWarrior:
		if st.Cond() == 11 && s.DropItems(p, brownScrollScrap, 1, 5, 500000) {
			st.SetCond(12)
		}
	case shackle1, shackle2:
		if st.Cond() == 16 && s.DropItems(p, shackleScalp, 1, 2, 500000) {
			if p.ItemCount(monsterEyeDestroyerSkin) == 5 && p.ItemCount(shamanNecklace) == 5 {
				st.SetCond(17)
			}
		}
	case monsterEyeDestroyer:
		if st.Cond() == 16 && s.DropItems(p, monsterEyeDestroyerSkin, 1, 5, 500000) {
			if p.ItemCount(shackleScalp) == 2 && p.ItemCount(shamanNecklace) == 5 {
				st.SetCond(17)
			}
		}
	case brekaOrcShaman:
		if st.Cond() == 16 && s.DropItems(p, shamanNecklace, 1, 5, 500000) {
			if p.ItemCount(shackleScalp) == 2 && p.ItemCount(monsterEyeDestroyerSkin) == 5 {
				st.SetCond(17)
			}
		}
	case grandis:
		if p.HasItem(triffRing) {
			s.DropItems(p, scriptureChapter3, 1, 1, 300000)
		}
	case medusa:
		if st.Cond() == 28 && s.DropItemsAlways(p, medusaBlood, 1, 12) {
			if p.ItemCount(ghoulSkin) == 10 && p.ItemCount(fetteredSoulIchor) == 5 && p.ItemCount(enchantedGargoyleNail) == 5 {
				st.SetCond(29)
			}
		}
	case ghoul:
		if st.Cond() == 28 && s.DropItemsAlways(p, ghoulSkin, 1, 10) {
			if p.ItemCount(medusaBlood) == 12 && p.ItemCount(fetteredSoulIchor) == 5 && p.ItemCount(enchantedGargoyleNail) == 5 {
				st.SetCond(29)
			}
		}
	case fetteredSoul:
		if st.Cond() == 28 && s.DropItemsAlways(p, fetteredSoulIchor, 1, 5) {
			if p.ItemCount(medusaBlood) == 12 && p.ItemCount(ghoulSkin) == 10 && p.ItemCount(enchantedGargoyleNail) == 5 {
				st.SetCond(29)
			}
		}
	case enchantedGargoyle:
		if st.Cond() == 28 && s.DropItemsAlways(p, enchantedGargoyleNail, 1, 5) {
			if p.ItemCount(medusaBlood) == 12 && p.ItemCount(ghoulSkin) == 10 && p.ItemCount(fetteredSoulIchor) == 5 {
				st.SetCond(29)
			}
		}
	}
}
