// Package q232 is the quest Test of the Lord, a second class-change test for
// orc shamans: Kakai sends the traveller to six elders for the remains of an
// ordeal, then to the ancestor Martankus for the Immortal Flame, for the Mark
// of Lord.
package q232

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q232_TestOfTheLord"

// Items.
const (
	ordealNecklace        = 3391
	varkeesCharm          = 3392
	tantusCharm           = 3393
	hatosCharm            = 3394
	takunaCharm           = 3395
	chiantaCharm          = 3396
	manakiasOrders        = 3397
	brekaOrcFang          = 3398
	manakiasAmulet        = 3399
	hugeOrcFang           = 3400
	sumarisLetter         = 3401
	urutuBlade            = 3402
	timakOrcSkull         = 3403
	swordIntoSkull        = 3404
	nerugaAxeBlade        = 3405
	axeOfCeremony         = 3406
	marshSpiderFeeler     = 3407
	marshSpiderFeet       = 3408
	handiworkSpiderBrooch = 3409
	monstereyeCornea      = 3410
	monstereyeWoodcarving = 3411
	bearFangNecklace      = 3412
	martankusCharm        = 3413
	ragnaOrcHead          = 3414
	ragnaChiefNotice      = 3415
	immortalFlame         = 3416
	boneArrow             = 1341
	adena                 = 57
)

// Rewards.
const markLord = 3390

// NPCs.
const (
	somak             = 30510
	manakia           = 30515
	jakal             = 30558
	sumari            = 30564
	kakai             = 30565
	varkees           = 30566
	tantus            = 30567
	hatos             = 30568
	takuna            = 30641
	chianta           = 30642
	firstOrc          = 30643
	ancestorMartankus = 30649
)

// The class the test is open to.
const classOrcShaman = 50

// quest holds the First Orc Martankus called, shared by every player: one at
// a time, until a talk to it starts the timer that sends it away.
type quest struct {
	firstOrc *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{firstOrc: new(script.NPCCell)}
	return script.Script{
		Title:   "Test of the Lord",
		QuestID: 232,
		Items: []int32{
			varkeesCharm, tantusCharm, hatosCharm, takunaCharm, chiantaCharm, manakiasOrders, brekaOrcFang,
			manakiasAmulet, hugeOrcFang, sumarisLetter, urutuBlade, timakOrcSkull, swordIntoSkull, nerugaAxeBlade,
			axeOfCeremony, marshSpiderFeeler, marshSpiderFeet, handiworkSpiderBrooch, monstereyeCornea,
			monstereyeWoodcarving, bearFangNecklace, martankusCharm, ragnaOrcHead, ragnaChiefNotice, immortalFlame,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {kakai},
			script.EventTalked: {
				kakai, chianta, hatos, somak, sumari, takuna, tantus, jakal, varkees, manakia, ancestorMartankus,
				firstOrc,
			},
			script.EventMyDying: {20233, 20269, 20270, 20564, 20583, 20584, 20585, 20586, 20587, 20588, 20778, 20779},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTimer: q.onTimer, OnTalk: q.onTalk, OnMyDying: onMyDying},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30565-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ordealNecklace, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30565-05b.htm"
		}
	case strings.EqualFold(e.Name, "30565-08.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, swordIntoSkull, 1)
		s.TakeItems(p, axeOfCeremony, 1)
		s.TakeItems(p, monstereyeWoodcarving, 1)
		s.TakeItems(p, handiworkSpiderBrooch, 1)
		s.TakeItems(p, ordealNecklace, 1)
		s.TakeItems(p, hugeOrcFang, 1)
		s.GiveItems(p, bearFangNecklace, 1)
	case strings.EqualFold(e.Name, "30566-02.htm"):
		s.GiveItems(p, varkeesCharm, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30567-02.htm"):
		s.GiveItems(p, tantusCharm, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30558-02.htm"):
		s.TakeItems(p, adena, 1000)
		s.GiveItems(p, nerugaAxeBlade, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30568-02.htm"):
		s.GiveItems(p, hatosCharm, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30641-02.htm"):
		s.GiveItems(p, takunaCharm, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30642-02.htm"):
		s.GiveItems(p, chiantaCharm, 1)
		s.PlaySound(p, script.SoundItemGet)
	case strings.EqualFold(e.Name, "30643-02.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.StartTimer("f_orc_despawn", nil, nil, firstOrcLifetime)
	case strings.EqualFold(e.Name, "30649-04.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bearFangNecklace, 1)
		s.GiveItems(p, martankusCharm, 1)
	case strings.EqualFold(e.Name, "30649-07.htm"):
		q.callFirstOrc(s)
	}
	return htmltext
}

// firstOrcLifetime is how long the First Orc stays once a traveller has
// talked to it.
const firstOrcLifetime = 10000 * time.Millisecond

// callFirstOrc places the First Orc when none stands; one another player's
// dialog placed meanwhile wins, and the extra one leaves.
func (q quest) callFirstOrc(s *script.Script) {
	if !q.firstOrc.Empty() {
		return
	}
	at := script.Loc{X: 21036, Y: -107690, Z: -3038, Heading: 200000}
	if n := s.Spawner().AddSpawn(firstOrc, at, false, 0); n != nil && !q.firstOrc.SetIfEmpty(n) {
		n.DeleteMe()
	}
}

// onTimer sends the First Orc away.
func (q quest) onTimer(_ *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "f_orc_despawn") {
		if o := q.firstOrc.Get(); o != nil {
			o.DeleteMe()
			q.firstOrc.Clear()
		}
	}
	return ""
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceOrc:
			htmltext = "30565-01.htm"
		case p.ClassID() != classOrcShaman:
			htmltext = "30565-02.htm"
		case p.Level() < 39:
			htmltext = "30565-03.htm"
		default:
			htmltext = "30565-04.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// handedIn answers an errand just done: the quest advances to its cond 2
// once the other five are held (done), else the traveller only hears of the
// item.
func handedIn(s *script.Script, p *script.Player, st *script.QuestState, done bool) {
	if done {
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	} else {
		s.PlaySound(p, script.SoundItemGet)
	}
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case varkees:
		switch {
		case p.HasItem(hugeOrcFang):
			htmltext = "30566-05.htm"
		case p.HasItem(varkeesCharm):
			if p.HasItem(manakiasAmulet) {
				htmltext = "30566-04.htm"
				s.TakeItems(p, varkeesCharm, -1)
				s.TakeItems(p, manakiasAmulet, -1)
				s.GiveItems(p, hugeOrcFang, 1)
				handedIn(s, p, st, p.HasItems(swordIntoSkull, axeOfCeremony, monstereyeWoodcarving, handiworkSpiderBrooch, ordealNecklace))
			} else {
				htmltext = "30566-03.htm"
			}
		default:
			htmltext = "30566-01.htm"
		}
	case manakia:
		switch {
		case p.HasItem(hugeOrcFang):
			htmltext = "30515-05.htm"
		case p.HasItem(manakiasAmulet):
			htmltext = "30515-04.htm"
		case p.HasItem(manakiasOrders):
			if p.ItemCount(brekaOrcFang) >= 20 {
				htmltext = "30515-03.htm"
				s.TakeItems(p, manakiasOrders, -1)
				s.TakeItems(p, brekaOrcFang, -1)
				s.GiveItems(p, manakiasAmulet, 1)
				s.PlaySound(p, script.SoundItemGet)
			} else {
				htmltext = "30515-02.htm"
			}
		default:
			htmltext = "30515-01.htm"
			s.GiveItems(p, manakiasOrders, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case tantus:
		switch {
		case p.HasItem(axeOfCeremony):
			htmltext = "30567-05.htm"
		case p.HasItem(tantusCharm):
			if p.ItemCount(boneArrow) >= 1000 {
				htmltext = "30567-04.htm"
				s.TakeItems(p, boneArrow, 1000)
				s.TakeItems(p, nerugaAxeBlade, 1)
				s.TakeItems(p, tantusCharm, 1)
				s.GiveItems(p, axeOfCeremony, 1)
				handedIn(s, p, st, p.HasItems(swordIntoSkull, monstereyeWoodcarving, handiworkSpiderBrooch, ordealNecklace, hugeOrcFang))
			} else {
				htmltext = "30567-03.htm"
			}
		default:
			htmltext = "30567-01.htm"
		}
	case jakal:
		switch {
		case p.HasItem(axeOfCeremony):
			htmltext = "30558-05.htm"
		case p.HasItem(nerugaAxeBlade):
			htmltext = "30558-04.htm"
		case p.HasItem(tantusCharm):
			if p.Adena() >= 1000 {
				htmltext = "30558-01.htm"
			} else {
				htmltext = "30558-03.htm"
			}
		}
	case hatos:
		switch {
		case p.HasItem(swordIntoSkull):
			htmltext = "30568-05.htm"
		case p.HasItem(hatosCharm):
			if p.HasItem(urutuBlade) && p.ItemCount(timakOrcSkull) >= 10 {
				htmltext = "30568-04.htm"
				s.TakeItems(p, hatosCharm, 1)
				s.TakeItems(p, urutuBlade, 1)
				s.TakeItems(p, timakOrcSkull, -1)
				s.GiveItems(p, swordIntoSkull, 1)
				handedIn(s, p, st, p.HasItems(axeOfCeremony, monstereyeWoodcarving, handiworkSpiderBrooch, ordealNecklace, hugeOrcFang))
			} else {
				htmltext = "30568-03.htm"
			}
		default:
			htmltext = "30568-01.htm"
		}
	case sumari:
		switch {
		case p.HasItem(urutuBlade):
			htmltext = "30564-03.htm"
		case p.HasItem(sumarisLetter):
			htmltext = "30564-02.htm"
		case p.HasItem(hatosCharm):
			htmltext = "30564-01.htm"
			s.GiveItems(p, sumarisLetter, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case somak:
		switch {
		case p.HasItem(swordIntoSkull):
			htmltext = "30510-03.htm"
		case p.HasItem(urutuBlade):
			htmltext = "30510-02.htm"
		case p.HasItem(sumarisLetter):
			htmltext = "30510-01.htm"
			s.TakeItems(p, sumarisLetter, 1)
			s.GiveItems(p, urutuBlade, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case takuna:
		switch {
		case p.HasItem(handiworkSpiderBrooch):
			htmltext = "30641-05.htm"
		case p.HasItem(takunaCharm):
			if p.ItemCount(marshSpiderFeeler) >= 10 && p.ItemCount(marshSpiderFeet) >= 10 {
				htmltext = "30641-04.htm"
				s.TakeItems(p, marshSpiderFeeler, -1)
				s.TakeItems(p, marshSpiderFeet, -1)
				s.TakeItems(p, takunaCharm, 1)
				s.GiveItems(p, handiworkSpiderBrooch, 1)
				handedIn(s, p, st, p.HasItems(swordIntoSkull, axeOfCeremony, monstereyeWoodcarving, ordealNecklace, hugeOrcFang))
			} else {
				htmltext = "30641-03.htm"
			}
		default:
			htmltext = "30641-01.htm"
		}
	case chianta:
		switch {
		case p.HasItem(monstereyeWoodcarving):
			htmltext = "30642-05.htm"
		case p.HasItem(chiantaCharm):
			if p.ItemCount(monstereyeCornea) >= 20 {
				htmltext = "30642-04.htm"
				s.TakeItems(p, monstereyeCornea, -1)
				s.TakeItems(p, chiantaCharm, 1)
				s.GiveItems(p, monstereyeWoodcarving, 1)
				handedIn(s, p, st, p.HasItems(swordIntoSkull, axeOfCeremony, handiworkSpiderBrooch, ordealNecklace, hugeOrcFang))
			} else {
				htmltext = "30642-03.htm"
			}
		default:
			htmltext = "30642-01.htm"
		}
	case kakai:
		switch {
		case cond == 1:
			htmltext = "30565-06.htm"
		case cond == 2:
			htmltext = "30565-07.htm"
		case cond == 3:
			htmltext = "30565-09.htm"
		case cond > 3 && cond < 7:
			htmltext = "30565-10.htm"
		case cond == 7:
			htmltext = "30565-11.htm"
			s.TakeItems(p, immortalFlame, 1)
			s.GiveItems(p, markLord, 1)
			s.RewardExpAndSp(p, 92955, 16250)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case ancestorMartankus:
		switch cond {
		case 3:
			htmltext = "30649-01.htm"
		case 4:
			htmltext = "30649-05.htm"
		case 5:
			htmltext = "30649-06.htm"
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, martankusCharm, 1)
			s.TakeItems(p, ragnaOrcHead, 1)
			s.TakeItems(p, ragnaChiefNotice, 1)
			s.GiveItems(p, immortalFlame, 1)
		case 6:
			htmltext = "30649-07.htm"
		case 7:
			htmltext = "30649-08.htm"
		}
	case firstOrc:
		switch cond {
		case 6:
			htmltext = "30643-01.htm"
		case 7:
			htmltext = "30643-03.htm"
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
	case 20564:
		if p.HasItem(chiantaCharm) {
			s.DropItemsAlways(p, monstereyeCornea, 1, 20)
		}
	case 20583, 20584, 20585:
		if p.HasItem(hatosCharm) {
			s.DropItems(p, timakOrcSkull, 1, 10, 710000)
		}
	case 20586:
		if p.HasItem(hatosCharm) {
			s.DropItems(p, timakOrcSkull, 1, 10, 810000)
		}
	case 20587, 20588:
		if p.HasItem(hatosCharm) {
			s.DropItemsAlways(p, timakOrcSkull, 1, 10)
		}
	case 20233:
		if p.HasItem(takunaCharm) {
			item := int32(marshSpiderFeeler)
			if p.ItemCount(marshSpiderFeeler) >= 10 {
				item = marshSpiderFeet
			}
			s.DropItemsAlways(p, item, 1, 10)
		}
	case 20269:
		if p.HasItem(manakiasOrders) {
			s.DropItems(p, brekaOrcFang, 1, 20, 410000)
		}
	case 20270:
		if p.HasItem(manakiasOrders) {
			s.DropItems(p, brekaOrcFang, 1, 20, 510000)
		}
	case 20778, 20779:
		if p.HasItem(martankusCharm) {
			if !p.HasItem(ragnaChiefNotice) {
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, ragnaChiefNotice, 1)
			} else if !p.HasItem(ragnaOrcHead) {
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, ragnaOrcHead, 1)
			}
		}
	}
}
