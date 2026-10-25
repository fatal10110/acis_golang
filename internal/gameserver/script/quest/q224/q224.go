// Package q224 is the quest Test of Sagittarius, a second class-change test
// for rogues, elven scouts and assassins: Bernard sends the traveller to
// Hamil, who wants hunter runes from the ants, then from the Breka orcs
// with Vokian's talisman, then sends it to Gauen for the parts of the
// Crescent Moon bow, and last to the Leto lizardmen, whose blood calls the
// serpent demon Kadesh, slain with that bow for the Mark of Sagittarius.
package q224

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q224_TestOfSagittarius"

// Items.
const (
	bernardIntroduction = 3294
	hamilLetter1        = 3295
	hamilLetter2        = 3296
	hamilLetter3        = 3297
	hunterRune1         = 3298
	hunterRune2         = 3299
	talismanOfKadesh    = 3300
	talismanOfSnake     = 3301
	mithrilClip         = 3302
	stakatoChitin       = 3303
	reinforcedBowstring = 3304
	manashenHorn        = 3305
	bloodOfLizardman    = 3306

	crescentMoonBow = 3028
	woodenArrow     = 17
)

// Rewards.
const markOfSagittarius = 3293

// NPCs.
const (
	bernard        = 30702
	hamil          = 30626
	sirAronTanford = 30653
	vokian         = 30514
	gauen          = 30717
)

// Monsters.
const (
	ant                   = 20079
	antCaptain            = 20080
	antOverseer           = 20081
	antRecruit            = 20082
	antPatrol             = 20084
	antGuard              = 20086
	nobleAnt              = 20089
	nobleAntLeader        = 20090
	brekaOrcShaman        = 20269
	brekaOrcOverlord      = 20270
	marshStakatoWorker    = 20230
	marshStakatoSoldier   = 20232
	marshStakatoDrone     = 20234
	marshSpider           = 20233
	roadScavenger         = 20551
	manashenGargoyle      = 20563
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
	serpentDemonKadesh    = 27090
)

// The classes the test is open to.
const (
	classRogue      = 7
	classElvenScout = 22
	classAssassin   = 35
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test Of Sagittarius",
		QuestID: 224,
		Items: []int32{
			bernardIntroduction, hamilLetter1, hamilLetter2, hamilLetter3, hunterRune1, hunterRune2,
			talismanOfKadesh, talismanOfSnake, mithrilClip, stakatoChitin, reinforcedBowstring, manashenHorn,
			bloodOfLizardman, crescentMoonBow,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {bernard},
			script.EventTalked:     {bernard, hamil, sirAronTanford, vokian, gauen},
			script.EventMyDying: {
				ant, antCaptain, antOverseer, antRecruit, antPatrol, antGuard, nobleAnt, nobleAntLeader,
				brekaOrcShaman, brekaOrcOverlord, marshStakatoWorker, marshStakatoSoldier, marshStakatoDrone,
				marshSpider, roadScavenger, manashenGargoyle, letoLizardman, letoLizardmanArcher,
				letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord,
				serpentDemonKadesh,
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
	// Bernard.
	case strings.EqualFold(e.Name, "30702-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, bernardIntroduction, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30702-04a.htm"
		}
	// Hamil.
	case strings.EqualFold(e.Name, "30626-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bernardIntroduction, 1)
		s.GiveItems(p, hamilLetter1, 1)
	case strings.EqualFold(e.Name, "30626-07.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hunterRune1, 10)
		s.GiveItems(p, hamilLetter2, 1)
	// Sir Aron Tanford.
	case strings.EqualFold(e.Name, "30653-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hamilLetter1, 1)
	// Vokian.
	case strings.EqualFold(e.Name, "30514-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hamilLetter2, 1)
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
		case class != classRogue && class != classElvenScout && class != classAssassin:
			htmltext = "30702-02.htm"
		case p.Level() < 39:
			htmltext = "30702-01.htm"
		default:
			htmltext = "30702-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case bernard:
			htmltext = "30702-05.htm"
		case hamil:
			htmltext = talkHamil(s, p, st, cond)
		case sirAronTanford:
			switch {
			case cond == 2:
				htmltext = "30653-01.htm"
			case cond > 2:
				htmltext = "30653-03.htm"
			}
		case vokian:
			switch {
			case cond == 5:
				htmltext = "30514-01.htm"
			case cond == 6:
				htmltext = "30514-03.htm"
			case cond == 7:
				htmltext = "30514-04.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, talismanOfSnake, 1)
			case cond > 7:
				htmltext = "30514-05.htm"
			}
		case gauen:
			switch {
			case cond == 9:
				htmltext = "30717-01.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, hamilLetter3, 1)
			case cond == 10:
				htmltext = "30717-03.htm"
			case cond == 11:
				htmltext = "30717-02.htm"
				st.SetCond(12)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, manashenHorn, 1)
				s.TakeItems(p, mithrilClip, 1)
				s.TakeItems(p, reinforcedBowstring, 1)
				s.TakeItems(p, stakatoChitin, 1)
				s.GiveItems(p, crescentMoonBow, 1)
				s.GiveItems(p, woodenArrow, 10)
			case cond > 11:
				htmltext = "30717-04.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkHamil is Hamil's page for a started test at cond, with the step it
// takes.
func talkHamil(s *script.Script, p *script.Player, st *script.QuestState, cond int32) string {
	switch {
	case cond == 1:
		return "30626-01.htm"
	case cond == 2 || cond == 3:
		return "30626-04.htm"
	case cond == 4:
		return "30626-05.htm"
	case cond > 4 && cond < 8:
		return "30626-08.htm"
	case cond == 8:
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hunterRune2, 10)
		s.GiveItems(p, hamilLetter3, 1)
		return "30626-09.htm"
	case cond > 8 && cond < 12:
		return "30626-10.htm"
	case cond == 12:
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
		return "30626-11.htm"
	case cond == 13:
		return "30626-12.htm"
	case cond == 14:
		s.TakeItems(p, bloodOfLizardman, -1)
		s.TakeItems(p, crescentMoonBow, 1)
		s.TakeItems(p, talismanOfKadesh, 1)
		s.GiveItems(p, markOfSagittarius, 1)
		s.RewardExpAndSp(p, 54726, 20250)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
		return "30626-13.htm"
	}
	return script.NoQuestMsg()
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case ant, antCaptain, antOverseer, antRecruit, antPatrol, antGuard, nobleAnt, nobleAntLeader:
		if st.Cond() == 3 && s.DropItems(p, hunterRune1, 1, 10, 500000) {
			st.SetCond(4)
		}
	case brekaOrcShaman, brekaOrcOverlord:
		if st.Cond() == 6 && s.DropItems(p, hunterRune2, 1, 10, 500000) {
			st.SetCond(7)
			s.GiveItems(p, talismanOfSnake, 1)
		}
	case marshStakatoWorker, marshStakatoSoldier, marshStakatoDrone:
		if st.Cond() == 10 && s.DropItems(p, stakatoChitin, 1, 1, 100000) && p.HasItems(manashenHorn, mithrilClip, reinforcedBowstring) {
			st.SetCond(11)
		}
	case marshSpider:
		if st.Cond() == 10 && s.DropItems(p, reinforcedBowstring, 1, 1, 100000) && p.HasItems(manashenHorn, mithrilClip, stakatoChitin) {
			st.SetCond(11)
		}
	case roadScavenger:
		if st.Cond() == 10 && s.DropItems(p, mithrilClip, 1, 1, 100000) && p.HasItems(manashenHorn, reinforcedBowstring, stakatoChitin) {
			st.SetCond(11)
		}
	case manashenGargoyle:
		if st.Cond() == 10 && s.DropItems(p, manashenHorn, 1, 1, 100000) && p.HasItems(reinforcedBowstring, mithrilClip, stakatoChitin) {
			st.SetCond(11)
		}
	case letoLizardman, letoLizardmanArcher, letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord:
		if st.Cond() == 13 {
			// Past 120 blood, each kill calls Kadesh 5% likelier.
			if (p.ItemCount(bloodOfLizardman)-120)*5 > int32(s.Rnd(100)) {
				s.PlaySound(p, script.SoundBeforeBattle)
				s.TakeItems(p, bloodOfLizardman, -1)
				s.Spawner().AddSpawn(serpentDemonKadesh, p, false, 300000*time.Millisecond)
			} else {
				s.DropItemsAlways(p, bloodOfLizardman, 1, 0)
			}
		}
	case serpentDemonKadesh:
		if st.Cond() == 13 {
			// Kadesh falls for good only to the Crescent Moon bow.
			if p.RightHandItemID() == crescentMoonBow {
				st.SetCond(14)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, talismanOfKadesh, 1)
			} else {
				s.Spawner().AddSpawn(serpentDemonKadesh, p, false, 300000*time.Millisecond)
			}
		}
	}
}
