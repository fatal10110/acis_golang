// Package q216 is the quest Trial of the Guildsman, a second class-change
// trial: Valkon sends a dwarf to Altran, Norman, Duning and Pinter, who want
// seven Journeyman Gems and seven Journeyman Deco Beads for the
// Journeyman Rings that earn the Mark of Guildsman.
package q216

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q216_TrialOfTheGuildsman"

// Items.
const (
	recipeJourneymanRing  = 3024
	recipeAmberBead       = 3025
	valkonRecommendation  = 3120
	mandragoraBerry       = 3121
	altranInstructions    = 3122
	altranRecommendation1 = 3123
	altranRecommendation2 = 3124
	normanInstructions    = 3125
	normanReceipt         = 3126
	duningInstructions    = 3127
	duningKey             = 3128
	normanList            = 3129
	grayBonePowder        = 3130
	graniteWhetstone      = 3131
	redPigment            = 3132
	braidedYarn           = 3133
	journeymanGem         = 3134
	pinterInstructions    = 3135
	amberBead             = 3136
	amberLump             = 3137
	journeymanDecoBeads   = 3138
	journeymanRing        = 3139
)

// Rewards.
const markOfGuildsman = 3119

const adena = 57

// NPCs.
const (
	valkon = 30103
	norman = 30210
	altran = 30283
	pinter = 30298
	duning = 30688
)

// Monsters.
const (
	ant               = 20079
	antCaptain        = 20080
	graniteGolem      = 20083
	mandragoraSprout  = 20154
	mandragoraSapling = 20155
	mandragoraBlossom = 20156
	silenos           = 20168
	strain            = 20200
	ghoul             = 20201
	deadSeeker        = 20202
	brekaOrcShaman    = 20269
	brekaOrcOverlord  = 20270
	brekaOrcWarrior   = 20271
)

// The classes the trial is open to.
const (
	classScavenger = 54
	classArtisan   = 56
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial of the Guildsman",
		QuestID: 216,
		Items: []int32{
			recipeJourneymanRing, recipeAmberBead, valkonRecommendation, mandragoraBerry, altranInstructions,
			altranRecommendation1, altranRecommendation2, normanInstructions, normanReceipt, duningInstructions,
			duningKey, normanList, grayBonePowder, graniteWhetstone, redPigment, braidedYarn, journeymanGem,
			pinterInstructions, amberBead, amberLump, journeymanDecoBeads, journeymanRing,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {valkon},
			script.EventTalked:     {valkon, norman, altran, pinter, duning},
			script.EventMyDying: {
				ant, antCaptain, graniteGolem, mandragoraSprout, mandragoraSapling, mandragoraBlossom, silenos,
				strain, ghoul, deadSeeker, brekaOrcShaman, brekaOrcOverlord, brekaOrcWarrior,
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
	case strings.EqualFold(e.Name, "30103-06.htm"):
		if p.Adena() >= 2000 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, adena, 2000)
			s.GiveItems(p, valkonRecommendation, 1)
			if secondclass.GiveDimensionalDiamonds35(s, p) {
				htmltext = "30103-06d.htm"
			}
		} else {
			htmltext = "30103-05a.htm"
		}
	case strings.EqualFold(e.Name, "30103-06c.htm") || strings.EqualFold(e.Name, "30103-07c.htm"):
		if st.Cond() < 3 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30103-09a.htm") || strings.EqualFold(e.Name, "30103-09b.htm"):
		s.TakeItems(p, altranInstructions, 1)
		s.TakeItems(p, journeymanRing, -1)
		s.GiveItems(p, markOfGuildsman, 1)
		s.RewardExpAndSp(p, 80993, 12250)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "30210-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, altranRecommendation1, 1)
		s.GiveItems(p, normanInstructions, 1)
		s.GiveItems(p, normanReceipt, 1)
	case strings.EqualFold(e.Name, "30210-10.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, normanList, 1)
	case strings.EqualFold(e.Name, "30283-03.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mandragoraBerry, 1)
		s.TakeItems(p, valkonRecommendation, 1)
		s.GiveItems(p, altranInstructions, 1)
		s.GiveItems(p, altranRecommendation1, 1)
		s.GiveItems(p, altranRecommendation2, 1)
		s.GiveItems(p, recipeJourneymanRing, 1)
	case strings.EqualFold(e.Name, "30298-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, altranRecommendation2, 1)
		s.GiveItems(p, pinterInstructions, 1)
		// An Artisan crafts the Amber Beads from a recipe; a Scavenger
		// spoils them off the ants instead.
		if p.ClassID() == classArtisan {
			htmltext = "30298-05.htm"
			s.GiveItems(p, recipeAmberBead, 1)
		}
	case strings.EqualFold(e.Name, "30688-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, normanReceipt, 1)
		s.GiveItems(p, duningInstructions, 1)
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
		case class != classScavenger && class != classArtisan:
			htmltext = "30103-01.htm"
		case p.Level() < 35:
			htmltext = "30103-02.htm"
		default:
			htmltext = "30103-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case valkon:
			switch {
			case cond == 1:
				htmltext = "30103-06c.htm"
			case cond < 5:
				htmltext = "30103-07.htm"
			case cond == 5:
				htmltext = "30103-08.htm"
			case cond == 6:
				if p.ItemCount(journeymanRing) == 7 {
					htmltext = "30103-09.htm"
				} else {
					htmltext = "30103-08.htm"
				}
			}
		case altran:
			switch {
			case cond < 4:
				htmltext = "30283-01.htm"
				if cond == 1 {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				}
			case cond == 4:
				htmltext = "30283-02.htm"
			default:
				htmltext = "30283-04.htm"
			}
		case norman:
			if cond == 5 {
				htmltext = talkNorman(s, st, p, htmltext)
			}
		case duning:
			if cond == 5 {
				switch {
				case p.HasItem(normanReceipt):
					htmltext = "30688-01.htm"
				case p.HasItem(duningInstructions):
					if p.ItemCount(duningKey) < 30 {
						htmltext = "30688-03.htm"
					} else {
						htmltext = "30688-04.htm"
						s.PlaySound(p, script.SoundItemGet)
						s.TakeItems(p, duningInstructions, 1)
					}
				default:
					htmltext = "30688-05.htm"
				}
			}
		case pinter:
			switch {
			case cond == 5:
				htmltext = talkPinter(s, st, p, htmltext)
			case p.HasItem(journeymanDecoBeads):
				htmltext = "30298-08.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkNorman is Norman's answer while the rings are made: his receipt and
// Duning's keys first, then the four materials of his list.
func talkNorman(s *script.Script, st *script.QuestState, p *script.Player, htmltext string) string {
	switch {
	case p.HasItem(altranRecommendation1):
		htmltext = "30210-01.htm"
	case p.HasItem(normanReceipt):
		htmltext = "30210-05.htm"
	case p.HasItem(duningInstructions):
		htmltext = "30210-06.htm"
	case p.ItemCount(duningKey) == 30:
		htmltext = "30210-07.htm"
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, duningKey, -1)
	case p.HasItem(normanList):
		if p.ItemCount(grayBonePowder) == 70 && p.ItemCount(graniteWhetstone) == 70 &&
			p.ItemCount(redPigment) == 70 && p.ItemCount(braidedYarn) == 70 {
			htmltext = "30210-12.htm"
			s.TakeItems(p, normanInstructions, 1)
			s.TakeItems(p, normanList, 1)
			s.TakeItems(p, braidedYarn, -1)
			s.TakeItems(p, graniteWhetstone, -1)
			s.TakeItems(p, grayBonePowder, -1)
			s.TakeItems(p, redPigment, -1)
			s.GiveItems(p, journeymanGem, 7)
			if p.ItemCount(journeymanDecoBeads) == 7 {
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		} else {
			htmltext = "30210-11.htm"
		}
	}
	return htmltext
}

// talkPinter is Pinter's answer while the rings are made: his instructions,
// then the seventy Amber Beads for the Journeyman Deco Beads.
func talkPinter(s *script.Script, st *script.QuestState, p *script.Player, htmltext string) string {
	switch {
	case p.HasItem(altranRecommendation2):
		if p.Level() < 36 {
			htmltext = "30298-01.htm"
		} else {
			htmltext = "30298-02.htm"
		}
	case p.HasItem(pinterInstructions):
		if p.ItemCount(amberBead) < 70 {
			htmltext = "30298-06.htm"
		} else {
			htmltext = "30298-07.htm"
			s.TakeItems(p, amberBead, -1)
			s.TakeItems(p, pinterInstructions, 1)
			s.GiveItems(p, journeymanDecoBeads, 7)
			if p.ItemCount(journeymanGem) == 7 {
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
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
	case mandragoraSprout, mandragoraSapling, mandragoraBlossom:
		if st.Cond() == 3 && s.DropItemsAlways(p, mandragoraBerry, 1, 1) {
			st.SetCond(4)
		}
	case brekaOrcWarrior, brekaOrcOverlord, brekaOrcShaman:
		if p.HasItem(duningInstructions) {
			s.DropItemsAlways(p, duningKey, 1, 30)
		}
	case ghoul, strain:
		if p.HasItem(normanList) {
			s.DropItemsAlways(p, grayBonePowder, 5, 70)
		}
	case graniteGolem:
		if p.HasItem(normanList) {
			s.DropItemsAlways(p, graniteWhetstone, 7, 70)
		}
	case deadSeeker:
		if p.HasItem(normanList) {
			s.DropItemsAlways(p, redPigment, 7, 70)
		}
	case silenos:
		if p.HasItem(normanList) {
			s.DropItemsAlways(p, braidedYarn, 10, 70)
		}
	case ant, antCaptain:
		if p.HasItem(pinterInstructions) {
			// A Scavenger who spoiled the ant gets twice the beads; an
			// Artisan may find an Amber Lump besides.
			count := int32(5)
			if p.ClassID() == classScavenger && e.NPC.ActualSpoiler(p) {
				count = 10
			}
			if s.DropItemsAlways(p, amberBead, count, 70) && p.ClassID() == classArtisan && s.RndBool() {
				s.GiveItems(p, amberLump, 1)
			}
		}
	}
}
