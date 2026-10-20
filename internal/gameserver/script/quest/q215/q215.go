// Package q215 is the quest Trial of the Pilgrim, a second class-change
// trial: Santiago sends a priest or shaman to Tanapi, Ancestor Martankus,
// Gauri Twinklerock, Dorf, Primos, Petron, Andellia, Uruha and Casian for
// the Mark of Pilgrim.
package q215

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q215_TrialOfThePilgrim"

// Items.
const (
	bookOfSage       = 2722
	voucherOfTrial   = 2723
	spiritOfFlame    = 2724
	essenceOfFlame   = 2725
	bookOfGerald     = 2726
	grayBadge        = 2727
	pictureOfNahir   = 2728
	hairOfNahir      = 2729
	statueOfEinhasad = 2730
	bookOfDarkness   = 2731
	debrisOfWillow   = 2732
	tagOfRumor       = 2733
)

// Rewards.
const markOfPilgrim = 2721

const adena = 57

// NPCs.
const (
	santiago          = 30648
	tanapi            = 30571
	ancestorMartankus = 30649
	gauriTwinklerock  = 30550
	dorf              = 30651
	gerald            = 30650
	primos            = 30117
	petron            = 30036
	andellia          = 30362
	uruha             = 30652
	casian            = 30612
)

// Monsters.
const (
	lavaSalamander = 27116
	nahir          = 27117
	blackWillow    = 27118
)

// The classes the trial is open to.
const (
	classCleric         = 15
	classElvenOracle    = 29
	classShillienOracle = 42
	classOrcShaman      = 50
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trial of the Pilgrim",
		QuestID: 215,
		Items: []int32{
			bookOfSage, voucherOfTrial, spiritOfFlame, essenceOfFlame, bookOfGerald, grayBadge, pictureOfNahir,
			hairOfNahir, statueOfEinhasad, bookOfDarkness, debrisOfWillow, tagOfRumor,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {santiago},
			script.EventTalked: {
				santiago, tanapi, ancestorMartankus, gauriTwinklerock, dorf, gerald, primos, petron, andellia, uruha, casian,
			},
			script.EventMyDying: {lavaSalamander, nahir, blackWillow},
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
	case strings.EqualFold(e.Name, "30648-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, voucherOfTrial, 1)
		if secondclass.GiveDimensionalDiamonds35(s, p) {
			htmltext = "30648-04a.htm"
		}
	case strings.EqualFold(e.Name, "30649-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, essenceOfFlame, 1)
		s.GiveItems(p, spiritOfFlame, 1)
	case strings.EqualFold(e.Name, "30650-02.htm"):
		if p.Adena() >= 100000 {
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, adena, 100000)
			s.GiveItems(p, bookOfGerald, 1)
		} else {
			htmltext = "30650-03.htm"
		}
	case strings.EqualFold(e.Name, "30652-02.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, debrisOfWillow, 1)
		s.GiveItems(p, bookOfDarkness, 1)
	case strings.EqualFold(e.Name, "30362-04.htm"):
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30362-05.htm"):
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bookOfDarkness, 1)
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
		case class != classCleric && class != classElvenOracle && class != classShillienOracle && class != classOrcShaman:
			htmltext = "30648-02.htm"
		case p.Level() < 35:
			htmltext = "30648-01.htm"
		default:
			htmltext = "30648-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case santiago:
			switch {
			case cond < 17:
				htmltext = "30648-09.htm"
			case cond == 17:
				htmltext = "30648-10.htm"
				s.TakeItems(p, bookOfSage, 1)
				s.GiveItems(p, markOfPilgrim, 1)
				s.RewardExpAndSp(p, 77382, 16000)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case tanapi:
			switch {
			case cond == 1:
				htmltext = "30571-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, voucherOfTrial, 1)
			case cond < 5:
				htmltext = "30571-02.htm"
			default:
				htmltext = "30571-03.htm"
				if cond == 5 {
					st.SetCond(6)
					s.PlaySound(p, script.SoundMiddle)
				}
			}
		case ancestorMartankus:
			switch cond {
			case 2:
				htmltext = "30649-01.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			case 3:
				htmltext = "30649-02.htm"
			case 4:
				htmltext = "30649-03.htm"
			}
		case gauriTwinklerock:
			switch {
			case cond == 6:
				htmltext = "30550-01.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, tagOfRumor, 1)
			case cond > 6:
				htmltext = "30550-02.htm"
			}
		case dorf:
			switch {
			case cond == 7:
				if !p.HasItem(bookOfGerald) {
					htmltext = "30651-01.htm"
				} else {
					htmltext = "30651-02.htm"
				}
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, tagOfRumor, 1)
				s.GiveItems(p, grayBadge, 1)
			case cond > 7:
				htmltext = "30651-03.htm"
			}
		case gerald:
			switch {
			case cond == 7 && !p.HasItem(bookOfGerald):
				htmltext = "30650-01.htm"
			case cond == 8 && p.HasItem(bookOfGerald):
				htmltext = "30650-04.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, bookOfGerald, 1)
				s.GiveItems(p, adena, 100000)
			}
		case primos:
			switch {
			case cond == 8:
				htmltext = "30117-01.htm"
				st.SetCond(9)
				s.PlaySound(p, script.SoundMiddle)
			case cond > 8:
				htmltext = "30117-02.htm"
			}
		case petron:
			switch {
			case cond == 9:
				htmltext = "30036-01.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, pictureOfNahir, 1)
			case cond == 10:
				htmltext = "30036-02.htm"
			case cond == 11:
				htmltext = "30036-03.htm"
				st.SetCond(12)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, hairOfNahir, 1)
				s.TakeItems(p, pictureOfNahir, 1)
				s.GiveItems(p, statueOfEinhasad, 1)
			case cond > 11:
				htmltext = "30036-04.htm"
			}
		case andellia:
			switch cond {
			case 12:
				if p.Level() < 36 {
					htmltext = "30362-01a.htm"
				} else {
					htmltext = "30362-01.htm"
					st.SetCond(13)
					s.PlaySound(p, script.SoundMiddle)
				}
			case 13:
				if s.RndBool() {
					htmltext = "30362-02.htm"
				} else {
					htmltext = "30362-02a.htm"
				}
			case 14:
				htmltext = "30362-07.htm"
			case 15:
				htmltext = "30362-03.htm"
			case 16:
				htmltext = "30362-06.htm"
			}
		case uruha:
			switch cond {
			case 14:
				htmltext = "30652-01.htm"
			case 15:
				htmltext = "30652-03.htm"
			}
		case casian:
			switch cond {
			case 16:
				htmltext = "30612-01.htm"
				st.SetCond(17)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bookOfDarkness, 1)
				s.TakeItems(p, grayBadge, 1)
				s.TakeItems(p, spiritOfFlame, 1)
				s.TakeItems(p, statueOfEinhasad, 1)
				s.GiveItems(p, bookOfSage, 1)
			case 17:
				htmltext = "30612-02.htm"
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
	case lavaSalamander:
		if st.Cond() == 3 && s.DropItems(p, essenceOfFlame, 1, 1, 200000) {
			st.SetCond(4)
		}
	case nahir:
		if st.Cond() == 10 && s.DropItems(p, hairOfNahir, 1, 1, 200000) {
			st.SetCond(11)
		}
	case blackWillow:
		if st.Cond() == 13 && s.DropItems(p, debrisOfWillow, 1, 1, 200000) {
			st.SetCond(14)
		}
	}
}
