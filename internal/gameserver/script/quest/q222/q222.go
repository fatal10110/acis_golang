// Package q222 is the quest Test of the Duelist, a second class-change test
// for fighters: Kaien sends the traveller for ten trophies of each of ten
// monsters across the kingdom, then three of each of five stronger ones,
// for the Mark of Duelist.
package q222

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q222_TestOfTheDuelist"

const kaien = 30623

// Items.
const (
	orderGludio       = 2763
	orderDion         = 2764
	orderGiran        = 2765
	orderOren         = 2766
	orderAden         = 2767
	puncherShard      = 2768
	nobleAntFeeler    = 2769
	droneChitin       = 2770
	deadSeekerFang    = 2771
	overlordNecklace  = 2772
	fetteredSoulChain = 2773
	chiefAmulet       = 2774
	enchantedEyeMeat  = 2775
	tamrinOrcRing     = 2776
	tamrinOrcArrow    = 2777
	finalOrder        = 2778
	excuroSkin        = 2779
	kratorShard       = 2780
	grandisSkin       = 2781
	timakOrcBelt      = 2782
	lakinMace         = 2783
)

// Rewards.
const markOfDuelist = 2762

// The trophies each part asks for, of each kind.
const (
	firstTrophyCount  = 10
	secondTrophyCount = 3
)

// The classes the test is open to.
const (
	classWarrior     = 0x01
	classOrcMonk     = 0x2f
	classElvenKnight = 0x13
	classPalusKnight = 0x20
)

// Monsters.
const (
	puncher               = 20085
	nobleAntLeader        = 20090
	marshStakatoDrone     = 20234
	deadSeeker            = 20202
	brekaOrcOverlord      = 20270
	fetteredSoul          = 20552
	letoLizardmanOverlord = 20582
	enchantedMonstereye   = 20564
	tamlinOrc             = 20601
	tamlinOrcArcher       = 20602
	excuro                = 20214
	krator                = 20217
	grandis               = 20554
	timakOrcOverlord      = 20588
	lakin                 = 20604
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test of the Duelist",
		QuestID: 222,
		Items: []int32{
			orderGludio, orderDion, orderGiran, orderOren, orderAden, finalOrder, puncherShard, nobleAntFeeler,
			droneChitin, deadSeekerFang, overlordNecklace, fetteredSoulChain, chiefAmulet, enchantedEyeMeat,
			tamrinOrcRing, tamrinOrcArrow, excuroSkin, kratorShard, grandisSkin, timakOrcBelt, lakinMace,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {kaien},
			script.EventTalked:     {kaien},
			script.EventMyDying: {
				puncher, nobleAntLeader, marshStakatoDrone, deadSeeker, brekaOrcOverlord, fetteredSoul,
				letoLizardmanOverlord, enchantedMonstereye, tamlinOrc, tamlinOrcArcher, excuro, krator, grandis,
				timakOrcOverlord, lakin,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// firstTrophies are the trophies of the first part, ten of each.
func firstTrophies() []int32 {
	return []int32{
		puncherShard, nobleAntFeeler, droneChitin, deadSeekerFang, overlordNecklace, fetteredSoulChain,
		chiefAmulet, enchantedEyeMeat, tamrinOrcRing, tamrinOrcArrow,
	}
}

// secondTrophies are the trophies of the second part, three of each.
func secondTrophies() []int32 {
	return []int32{excuroSkin, kratorShard, grandisSkin, timakOrcBelt, lakinMace}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30623-04.htm"):
		if p.Race() == player.RaceOrc {
			htmltext = "30623-05.htm"
		}
	case strings.EqualFold(e.Name, "30623-07.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.SetCond(2)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, orderGludio, 1)
		s.GiveItems(p, orderDion, 1)
		s.GiveItems(p, orderGiran, 1)
		s.GiveItems(p, orderOren, 1)
		s.GiveItems(p, orderAden, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30623-07a.htm"
		}
	case strings.EqualFold(e.Name, "30623-16.htm"):
		if st.Cond() == 3 {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, orderGludio, 1)
			s.TakeItems(p, orderDion, 1)
			s.TakeItems(p, orderGiran, 1)
			s.TakeItems(p, orderOren, 1)
			s.TakeItems(p, orderAden, 1)
			for _, id := range firstTrophies() {
				s.TakeItems(p, id, -1)
			}
			s.GiveItems(p, finalOrder, 1)
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
		switch class := p.ClassID(); {
		case class != classWarrior && class != classOrcMonk && class != classElvenKnight && class != classPalusKnight:
			htmltext = "30623-02.htm"
		case p.Level() < 39:
			htmltext = "30623-01.htm"
		default:
			htmltext = "30623-03.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 2:
			htmltext = "30623-07a.htm"
		case 3:
			htmltext = "30623-13.htm"
		case 4:
			htmltext = "30623-17.htm"
		case 5:
			htmltext = "30623-18.htm"
			s.TakeItems(p, finalOrder, 1)
			for _, id := range secondTrophies() {
				s.TakeItems(p, id, -1)
			}
			s.GiveItems(p, markOfDuelist, 1)
			s.RewardExpAndSp(p, 47015, 20000)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
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
	switch st.Cond() {
	case 2:
		trophy := firstTrophyOf(e.NPC.NpcID())
		if trophy != 0 && s.DropItemsAlways(p, trophy, 1, firstTrophyCount) && holdsAll(p, firstTrophies(), firstTrophyCount) {
			st.SetCond(3)
		}
	case 4:
		trophy := secondTrophyOf(e.NPC.NpcID())
		if trophy != 0 && s.DropItemsAlways(p, trophy, 1, secondTrophyCount) && holdsAll(p, secondTrophies(), secondTrophyCount) {
			st.SetCond(5)
		}
	}
}

// firstTrophyOf returns the first part's trophy the monster npcID drops, 0
// for none.
func firstTrophyOf(npcID int32) int32 {
	switch npcID {
	case puncher:
		return puncherShard
	case nobleAntLeader:
		return nobleAntFeeler
	case marshStakatoDrone:
		return droneChitin
	case deadSeeker:
		return deadSeekerFang
	case brekaOrcOverlord:
		return overlordNecklace
	case fetteredSoul:
		return fetteredSoulChain
	case letoLizardmanOverlord:
		return chiefAmulet
	case enchantedMonstereye:
		return enchantedEyeMeat
	case tamlinOrc:
		return tamrinOrcRing
	case tamlinOrcArcher:
		return tamrinOrcArrow
	}
	return 0
}

// secondTrophyOf returns the second part's trophy the monster npcID drops,
// 0 for none.
func secondTrophyOf(npcID int32) int32 {
	switch npcID {
	case excuro:
		return excuroSkin
	case krator:
		return kratorShard
	case lakin:
		return lakinMace
	case grandis:
		return grandisSkin
	case timakOrcOverlord:
		return timakOrcBelt
	}
	return 0
}

// holdsAll reports whether p holds at least count of each of ids.
func holdsAll(p *script.Player, ids []int32, count int32) bool {
	for _, id := range ids {
		if p.ItemCount(id) < count {
			return false
		}
	}
	return true
}
