// Package q333 is the quest Hunt of the Black Lion: Sophya of the Black Lion
// mercenaries sends a marked mercenary to four hunting grounds for proof of
// the kills, pays for the proof in adena and lion claws, and trades the claws
// for supplies; the cargo boxes the monsters drop open at Redfoot for trade
// goods and relic pieces, which Rupio, Undrias and Lockirin buy back.
package q333

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q333_HuntOfTheBlackLion"

// NPCs.
const (
	sophya   = 30735
	redfoot  = 30736
	rupio    = 30471
	undrias  = 30130
	lockirin = 30531
	morgan   = 30737
)

// blackLionMark is the item a mercenary needs to start.
const blackLionMark = 1369

// Quest items.
const (
	lionClaw          = 3675
	lionEye           = 3676
	guildCoin         = 3677
	undeadAsh         = 3848
	bloodyAxeInsignia = 3849
	deluFang          = 3850
	stakatoTalon      = 3851
	sophyaLetter1     = 3671
	sophyaLetter2     = 3672
	sophyaLetter3     = 3673
	sophyaLetter4     = 3674
)

// Cargo boxes and what they open into.
const (
	cargoBox1          = 3440
	cargoBox2          = 3441
	cargoBox3          = 3442
	cargoBox4          = 3443
	gludioApple        = 3444
	cornMeal           = 3445
	wolfPelts          = 3446
	moonstone          = 3447
	gludioWheatFlower  = 3448
	spidersilkRope     = 3449
	alexandrite        = 3450
	silverTea          = 3451
	golemPart          = 3452
	fireEmerald        = 3453
	silkFrock          = 3454
	porcelanUrn        = 3455
	imperialDiamond    = 3456
	statueShilienHead  = 3457
	statueShilienTorso = 3458
	statueShilienArm   = 3459
	statueShilienLeg   = 3460
	completeStatue     = 3461
	tabletFragment1    = 3462
	tabletFragment2    = 3463
	tabletFragment3    = 3464
	tabletFragment4    = 3465
	completeTablet     = 3466
)

// Neutral items.
const (
	adena             = 57
	swiftAttackPotion = 735
	scrollOfEscape    = 736
	healingPotion     = 1061
	soulshotD         = 1463
	spiritshotD       = 2510
)

// drop is one monster's row of the hunt: the letter that sends the player
// to its ground, the proof it drops and its chance, the cargo box it drops
// and its chance.
type drop struct {
	letter, npcID, item, itemChance, box, boxChance int32
}

// dropList returns the hunt's monsters, ground by ground.
func dropList() []drop {
	return []drop{
		// Part #1 - Execution Ground
		{sophyaLetter1, 20160, undeadAsh, 500000, cargoBox1, 90000},  // Neer Crawler
		{sophyaLetter1, 20171, undeadAsh, 500000, cargoBox1, 60000},  // Specter
		{sophyaLetter1, 20197, undeadAsh, 500000, cargoBox1, 70000},  // Sorrow Maiden
		{sophyaLetter1, 20198, undeadAsh, 500000, cargoBox1, 80000},  // Neer Ghoul Berserker
		{sophyaLetter1, 20200, undeadAsh, 500000, cargoBox1, 100000}, // Strain
		{sophyaLetter1, 20201, undeadAsh, 500000, cargoBox1, 110000}, // Ghoul

		// Part #2 - Partisan Hideaway
		{sophyaLetter2, 20207, bloodyAxeInsignia, 500000, cargoBox2, 60000},  // Ol Mahum Guerilla
		{sophyaLetter2, 20208, bloodyAxeInsignia, 500000, cargoBox2, 70000},  // Ol Mahum Raider
		{sophyaLetter2, 20209, bloodyAxeInsignia, 500000, cargoBox2, 80000},  // Ol Mahum Marksman
		{sophyaLetter2, 20210, bloodyAxeInsignia, 500000, cargoBox2, 90000},  // Ol Mahum Sergeant
		{sophyaLetter2, 20211, bloodyAxeInsignia, 500000, cargoBox2, 100000}, // Ol Mahum Captain

		// Part #3 - Near Giran Town
		{sophyaLetter3, 20251, deluFang, 500000, cargoBox3, 100000}, // Delu Lizardman
		{sophyaLetter3, 20252, deluFang, 500000, cargoBox3, 110000}, // Delu Lizardman Scout
		{sophyaLetter3, 20253, deluFang, 500000, cargoBox3, 120000}, // Delu Lizardman Warrior

		// Part #4 - Cruma Area
		{sophyaLetter4, 20157, stakatoTalon, 500000, cargoBox4, 100000}, // Marsh Stakato
		{sophyaLetter4, 20230, stakatoTalon, 500000, cargoBox4, 110000}, // Marsh Stakato Worker
		{sophyaLetter4, 20232, stakatoTalon, 500000, cargoBox4, 120000}, // Marsh Stakato Soldier
		{sophyaLetter4, 20234, stakatoTalon, 500000, cargoBox4, 130000}, // Marsh Stakato Drone
	}
}

// New returns the quest.
func New() script.Script {
	drops := dropList()
	dying := make([]int32, 0, len(drops))
	for _, d := range drops {
		dying = append(dying, d.npcID)
	}
	return script.Script{
		Title:   "Hunt Of The Black Lion",
		QuestID: 333,
		Items:   []int32{lionClaw, lionEye, guildCoin, undeadAsh, bloodyAxeInsignia, deluFang, stakatoTalon, sophyaLetter1, sophyaLetter2, sophyaLetter3, sophyaLetter4},
		Bind: script.Bindings{
			script.EventQuestStart: {sophya},
			script.EventTalked:     {sophya, redfoot, rupio, undrias, lockirin, morgan},
			script.EventMyDying:    dying,
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
	case strings.EqualFold(e.Name, "30735-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30735-10.htm"):
		giveLetter(s, p, sophyaLetter1)
	case strings.EqualFold(e.Name, "30735-11.htm"):
		giveLetter(s, p, sophyaLetter2)
	case strings.EqualFold(e.Name, "30735-12.htm"):
		giveLetter(s, p, sophyaLetter3)
	case strings.EqualFold(e.Name, "30735-13.htm"):
		giveLetter(s, p, sophyaLetter4)
	case strings.EqualFold(e.Name, "30735-16.htm"):
		if p.ItemCount(lionClaw) > 9 {
			htmltext = tradeClaws(s, p)
		}
	case strings.EqualFold(e.Name, "30735-20.htm"):
		s.TakeItems(p, sophyaLetter1, -1)
		s.TakeItems(p, sophyaLetter2, -1)
		s.TakeItems(p, sophyaLetter3, -1)
		s.TakeItems(p, sophyaLetter4, -1)
	case strings.EqualFold(e.Name, "30735-26.htm"):
		s.TakeItems(p, lionClaw, -1)
		s.TakeItems(p, lionEye, -1)
		s.TakeItems(p, guildCoin, -1)
		s.TakeItems(p, blackLionMark, -1)
		s.TakeItems(p, sophyaLetter1, -1)
		s.TakeItems(p, sophyaLetter2, -1)
		s.TakeItems(p, sophyaLetter3, -1)
		s.TakeItems(p, sophyaLetter4, -1)
		s.GiveItems(p, adena, 12400)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30736-03.htm"):
		htmltext = openCargoBox(s, p)
	case strings.EqualFold(e.Name, "30736-07.htm"):
		state := st.GetInt("state")
		if p.Adena() > 200+state*200 {
			if state < 3 {
				htmltext = redfootGossip(s.Rnd(100))
				s.TakeItems(p, adena, 200+state*200)
				st.Set("state", itoa(state+1))
			} else {
				htmltext = "30736-08.htm"
			}
		}
	case strings.EqualFold(e.Name, "30471-03.htm"):
		if p.HasItems(statueShilienHead, statueShilienTorso, statueShilienArm, statueShilienLeg) {
			s.TakeItems(p, statueShilienHead, 1)
			s.TakeItems(p, statueShilienTorso, 1)
			s.TakeItems(p, statueShilienArm, 1)
			s.TakeItems(p, statueShilienLeg, 1)
			if s.RndBool() {
				htmltext = "30471-04.htm"
				s.GiveItems(p, completeStatue, 1)
			} else {
				htmltext = "30471-05.htm"
			}
		}
	case strings.EqualFold(e.Name, "30471-06.htm"):
		if p.HasItems(tabletFragment1, tabletFragment2, tabletFragment3, tabletFragment4) {
			s.TakeItems(p, tabletFragment1, 1)
			s.TakeItems(p, tabletFragment2, 1)
			s.TakeItems(p, tabletFragment3, 1)
			s.TakeItems(p, tabletFragment4, 1)
			if s.RndBool() {
				htmltext = "30471-07.htm"
				s.GiveItems(p, completeTablet, 1)
			} else {
				htmltext = "30471-08.htm"
			}
		}
	case strings.EqualFold(e.Name, "30130-04.htm") && p.HasItem(completeStatue):
		s.TakeItems(p, completeStatue, 1)
		s.GiveItems(p, adena, 30000)
	case strings.EqualFold(e.Name, "30531-04.htm") && p.HasItem(completeTablet):
		s.TakeItems(p, completeTablet, 1)
		s.GiveItems(p, adena, 30000)
	case strings.EqualFold(e.Name, "30737-06.htm"):
		htmltext = sellCargoBox(s, p, htmltext)
	}
	return htmltext
}

// giveLetter hands out one of Sophya's letters, unless the player holds it.
func giveLetter(s *script.Script, p *script.Player, letter int32) {
	if !p.HasItem(letter) {
		s.GiveItems(p, letter, 1)
		s.PlaySound(p, script.SoundItemGet)
	}
}

// tradeClaws takes ten lion claws for a lion eye, while the player holds
// fewer than nine, and a supply pack that grows with the eyes held.
func tradeClaws(s *script.Script, p *script.Player) string {
	s.TakeItems(p, lionClaw, 10)
	eyes := p.ItemCount(lionEye)
	switch {
	case eyes < 5:
		s.GiveItems(p, lionEye, 1)
		random := s.Rnd(100)
		switch {
		case random < 25:
			s.GiveItems(p, healingPotion, 20)
		case random < 50:
			giveShots(s, p, 50, 100)
		case random < 75:
			s.GiveItems(p, scrollOfEscape, 20)
		default:
			s.GiveItems(p, swiftAttackPotion, 3)
		}
		return "30735-17a.htm"
	case eyes < 9:
		s.GiveItems(p, lionEye, 1)
		random := s.Rnd(100)
		switch {
		case random < 25:
			s.GiveItems(p, healingPotion, 25)
		case random < 50:
			giveShots(s, p, 100, 200)
		case random < 75:
			s.GiveItems(p, scrollOfEscape, 20)
		default:
			s.GiveItems(p, swiftAttackPotion, 3)
		}
		return "30735-18b.htm"
	default:
		random := s.Rnd(100)
		switch {
		case random < 25:
			s.GiveItems(p, healingPotion, 50)
		case random < 50:
			giveShots(s, p, 200, 400)
		case random < 75:
			s.GiveItems(p, scrollOfEscape, 30)
		default:
			s.GiveItems(p, swiftAttackPotion, 4)
		}
		return "30735-19b.htm"
	}
}

// giveShots gives a mage the spiritshots, anyone else the soulshots.
func giveShots(s *script.Script, p *script.Player, spiritshots, soulshots int32) {
	if p.IsMageClass() {
		s.GiveItems(p, spiritshotD, spiritshots)
	} else {
		s.GiveItems(p, soulshotD, soulshots)
	}
}

// takeCargoBox takes one cargo box, the lowest numbered the player holds.
func takeCargoBox(s *script.Script, p *script.Player, cargo1, cargo2, cargo3 bool) {
	switch {
	case cargo1:
		s.TakeItems(p, cargoBox1, 1)
	case cargo2:
		s.TakeItems(p, cargoBox2, 1)
	case cargo3:
		s.TakeItems(p, cargoBox3, 1)
	default:
		s.TakeItems(p, cargoBox4, 1)
	}
}

// openCargoBox has Redfoot open a cargo box for 650 adena: two hundred-sided
// rolls pick the trade good, or a statue or tablet piece.
func openCargoBox(s *script.Script, p *script.Player) string {
	cargo1 := p.HasItem(cargoBox1)
	cargo2 := p.HasItem(cargoBox2)
	cargo3 := p.HasItem(cargoBox3)
	cargo4 := p.HasItem(cargoBox4)
	if !(cargo1 || cargo2 || cargo3 || cargo4) || p.Adena() <= 649 {
		return "30736-05.htm"
	}
	s.TakeItems(p, adena, 650)
	takeCargoBox(s, p, cargo1, cargo2, cargo3)

	i0 := s.Rnd(100)
	i1 := s.Rnd(100)
	pick := func(a, b, c int32, pageA, pageB, pageC string) string {
		switch {
		case i1 < 33:
			s.GiveItems(p, a, 1)
			return pageA
		case i1 < 66:
			s.GiveItems(p, b, 1)
			return pageB
		default:
			s.GiveItems(p, c, 1)
			return pageC
		}
	}
	quarter := func(a, b, c, d int32) {
		switch {
		case i1 < 25:
			s.GiveItems(p, a, 1)
		case i1 < 50:
			s.GiveItems(p, b, 1)
		case i1 < 75:
			s.GiveItems(p, c, 1)
		default:
			s.GiveItems(p, d, 1)
		}
	}
	switch {
	case i0 < 40:
		return pick(gludioApple, cornMeal, wolfPelts, "30736-04a.htm", "30736-04b.htm", "30736-04c.htm")
	case i0 < 60:
		return pick(moonstone, gludioWheatFlower, spidersilkRope, "30736-04d.htm", "30736-04e.htm", "30736-04f.htm")
	case i0 < 70:
		return pick(alexandrite, silverTea, golemPart, "30736-04g.htm", "30736-04h.htm", "30736-04i.htm")
	case i0 < 75:
		return pick(fireEmerald, silkFrock, porcelanUrn, "30736-04j.htm", "30736-04k.htm", "30736-04l.htm")
	case i0 < 76:
		s.GiveItems(p, imperialDiamond, 1)
		return "30736-04m.htm"
	case s.RndBool():
		quarter(statueShilienHead, statueShilienTorso, statueShilienArm, statueShilienLeg)
		return "30736-04n.htm"
	default:
		quarter(tabletFragment1, tabletFragment2, tabletFragment3, tabletFragment4)
		return "30736-04o.htm"
	}
}

// redfootGossip returns the page of Redfoot's gossip a hundred-sided roll
// picks, one of twenty.
func redfootGossip(i0 int) string {
	switch {
	case i0 < 5:
		return "30736-08a.htm"
	case i0 < 10:
		return "30736-08b.htm"
	case i0 < 15:
		return "30736-08c.htm"
	case i0 < 20:
		return "30736-08d.htm"
	case i0 < 25:
		return "30736-08e.htm"
	case i0 < 30:
		return "30736-08f.htm"
	case i0 < 35:
		return "30736-08g.htm"
	case i0 < 40:
		return "30736-08h.htm"
	case i0 < 45:
		return "30736-08i.htm"
	case i0 < 50:
		return "30736-08j.htm"
	case i0 < 55:
		return "30736-08k.htm"
	case i0 < 60:
		return "30736-08l.htm"
	case i0 < 65:
		return "30736-08m.htm"
	case i0 < 70:
		return "30736-08n.htm"
	case i0 < 75:
		return "30736-08o.htm"
	case i0 < 80:
		return "30736-08p.htm"
	case i0 < 85:
		return "30736-08q.htm"
	case i0 < 90:
		return "30736-08r.htm"
	case i0 < 95:
		return "30736-08s.htm"
	default:
		return "30736-08t.htm"
	}
}

// sellCargoBox has Morgan buy a cargo box: the pay grows with the guild
// coins held, and a coin comes with it until the player holds eighty.
// htmltext is the answer when the player holds no box.
func sellCargoBox(s *script.Script, p *script.Player, htmltext string) string {
	cargo1 := p.HasItem(cargoBox1)
	cargo2 := p.HasItem(cargoBox2)
	cargo3 := p.HasItem(cargoBox3)
	cargo4 := p.HasItem(cargoBox4)
	if !(cargo1 || cargo2 || cargo3 || cargo4) {
		return htmltext
	}
	takeCargoBox(s, p, cargo1, cargo2, cargo3)

	coins := p.ItemCount(guildCoin)
	switch {
	case coins < 40:
		htmltext = "30737-03.htm"
		s.GiveItems(p, adena, 100)
	case coins < 80:
		htmltext = "30737-04.htm"
		s.GiveItems(p, adena, 200)
	default:
		htmltext = "30737-05.htm"
		s.GiveItems(p, adena, 300)
	}
	if coins < 80 {
		s.GiveItems(p, guildCoin, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Level() < 25:
			htmltext = "30735-01.htm"
		case !p.HasItem(blackLionMark):
			htmltext = "30735-02.htm"
		default:
			htmltext = "30735-03.htm"
		}
	case questlog.StatusStarted:
		hasCargo := p.HasAtLeastOneItem(cargoBox1, cargoBox2, cargoBox3, cargoBox4)
		hasStatue := p.HasItems(statueShilienHead, statueShilienTorso, statueShilienArm, statueShilienLeg)
		hasTablet := p.HasItems(tabletFragment1, tabletFragment2, tabletFragment3, tabletFragment4)
		switch e.NPC.NpcID() {
		case sophya:
			htmltext = talkSophya(s, p)
		case redfoot:
			htmltext = pageIf(hasCargo, "30736-02.htm", "30736-01.htm")
		case rupio:
			htmltext = pageIf(hasStatue || hasTablet, "30471-02.htm", "30471-01.htm")
		case undrias:
			if !p.HasItem(completeStatue) {
				htmltext = pageIf(hasStatue, "30130-02.htm", "30130-01.htm")
			} else {
				htmltext = "30130-03.htm"
			}
		case lockirin:
			if !p.HasItem(completeTablet) {
				htmltext = pageIf(hasTablet, "30531-02.htm", "30531-01.htm")
			} else {
				htmltext = "30531-03.htm"
			}
		case morgan:
			htmltext = pageIf(hasCargo, "30737-02.htm", "30737-01.htm")
		}
	}
	return htmltext
}

// talkSophya pays for the proof of the hunt: 35 adena a piece, and lion
// claws for twenty pieces and more.
func talkSophya(s *script.Script, p *script.Player) string {
	if !p.HasAtLeastOneItem(sophyaLetter1, sophyaLetter2, sophyaLetter3, sophyaLetter4) {
		return "30735-14.htm"
	}
	if !p.HasAtLeastOneItem(undeadAsh, bloodyAxeInsignia, deluFang, stakatoTalon) {
		return pageIf(p.HasAtLeastOneItem(cargoBox1, cargoBox2, cargoBox3, cargoBox4), "30735-15a.htm", "30735-15.htm")
	}
	count := p.ItemCount(undeadAsh) + p.ItemCount(bloodyAxeInsignia) + p.ItemCount(deluFang) + p.ItemCount(stakatoTalon)

	s.TakeItems(p, undeadAsh, -1)
	s.TakeItems(p, bloodyAxeInsignia, -1)
	s.TakeItems(p, deluFang, -1)
	s.TakeItems(p, stakatoTalon, -1)
	s.GiveItems(p, adena, count*35)

	switch {
	case count >= 20 && count < 50:
		s.GiveItems(p, lionClaw, 1)
	case count >= 50 && count < 100:
		s.GiveItems(p, lionClaw, 2)
	case count >= 100:
		s.GiveItems(p, lionClaw, 3)
	}
	return pageIf(p.HasAtLeastOneItem(cargoBox1, cargoBox2, cargoBox3, cargoBox4), "30735-23.htm", "30735-22.htm")
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	npcID := e.NPC.NpcID()
	for _, d := range dropList() {
		if p.HasItem(d.letter) && npcID == d.npcID {
			s.DropItems(p, d.item, 1, 0, d.itemChance)
			s.DropItems(p, d.box, 1, 0, d.boxChance)
			break
		}
	}
}

// pageIf returns yes when cond holds, else no.
func pageIf(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
