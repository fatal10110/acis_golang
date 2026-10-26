// Package q420 is the quest Little Wing: a young adventurer has a fairy
// stone made, trades it for fairy dust or keeps it, feeds a dragon egg to the
// fairy and is given a dragon flute.
package q420

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q420_LittleWing"

// Needed items.
const (
	coal          = 1870
	charcoal      = 1871
	silverNugget  = 1873
	stoneOfPurity = 1875
	gemstoneD     = 2130
	gemstoneC     = 2131
)

// Quest items.
const (
	fairyDust            = 3499
	fairyStone           = 3816
	deluxeFairyStone     = 3817
	fairyStoneList       = 3818
	deluxeFairyStoneList = 3819
	toadLordBackSkin     = 3820
	juiceOfMonkshood     = 3821
	scaleOfDrakeExarion  = 3822
	eggOfDrakeExarion    = 3823
	scaleOfDrakeZwov     = 3824
	eggOfDrakeZwov       = 3825
	scaleOfDrakeKalibran = 3826
	eggOfDrakeKalibran   = 3827
	scaleOfWyvernSuzet   = 3828
	eggOfWyvernSuzet     = 3829
	scaleOfWyvernShamhai = 3830
	eggOfWyvernShamhai   = 3831
)

// Rewards.
const (
	dragonfluteOfWind     = 3500
	dragonfluteOfStar     = 3501
	dragonfluteOfTwilight = 3502
	hatchlingSoftLeather  = 3912
	foodForHatchling      = 4038
)

// Npcs.
const (
	maria    = 30608
	cronos   = 30610
	byron    = 30711
	mimyu    = 30747
	exarion  = 30748
	zwov     = 30749
	kalibran = 30750
	suzet    = 30751
	shamhai  = 30752
	cooper   = 30829
)

// What the monsters say when the egg they dropped is not the last one needed,
// and when they take the deluxe fairy stone.
const (
	sayKalibran = 42046
	saySuzet    = 42047
	sayStone    = 42048
	sayExarion  = 42049
)

// quest holds how many times Mimyu was talked to: it moves to another of its
// places every time.
type quest struct{ counter *script.IntCell }

// New returns the quest.
func New() script.Script {
	q := quest{counter: new(script.IntCell)}
	return script.Script{
		Title:   "Little Wing",
		QuestID: 420,
		Items: []int32{
			fairyStone, deluxeFairyStone, fairyStoneList, deluxeFairyStoneList, toadLordBackSkin, juiceOfMonkshood,
			scaleOfDrakeExarion, eggOfDrakeExarion, scaleOfDrakeZwov, eggOfDrakeZwov, scaleOfDrakeKalibran,
			eggOfDrakeKalibran, scaleOfWyvernSuzet, eggOfWyvernSuzet, scaleOfWyvernShamhai, eggOfWyvernShamhai,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {cooper, mimyu},
			script.EventTalked:     {maria, cronos, byron, mimyu, exarion, zwov, kalibran, suzet, shamhai, cooper},
			script.EventMyDying: {
				20202, 20231, 20233, 20270, 20551, 20580, 20589, 20590, 20591, 20592, 20593, 20594, 20595, 20596,
				20597, 20598, 20599,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: q.onTalk, OnMyDying: onMyDying},
	}
}

// mimyuLoc returns the n-th place Mimyu moves to.
func mimyuLoc(n int) script.Loc {
	switch n {
	case 0:
		return script.Loc{X: 109816, Y: 40854, Z: -4640}
	case 1:
		return script.Loc{X: 108940, Y: 41615, Z: -4643}
	}
	return script.Loc{X: 110395, Y: 41625, Z: -4642}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Cooper
	case strings.EqualFold(e.Name, "30829-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	// Cronos
	case strings.EqualFold(e.Name, "30610-05.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, fairyStoneList, 1)
	case strings.EqualFold(e.Name, "30610-06.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, deluxeFairyStoneList, 1)
	case strings.EqualFold(e.Name, "30610-12.htm"):
		st.SetCond(2)
		st.Set("deluxestone", "1")
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, fairyStoneList, 1)
	case strings.EqualFold(e.Name, "30610-13.htm"):
		st.SetCond(2)
		st.Set("deluxestone", "1")
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, deluxeFairyStoneList, 1)
	// Maria
	case strings.EqualFold(e.Name, "30608-03.htm"):
		if !checkItems(p, false) {
			htmltext = "30608-01.htm" // Avoid to continue while trade or drop mats before clicking bypass
		} else {
			s.TakeItems(p, coal, 10)
			s.TakeItems(p, charcoal, 10)
			s.TakeItems(p, gemstoneD, 1)
			s.TakeItems(p, silverNugget, 3)
			s.TakeItems(p, toadLordBackSkin, -1)
			s.TakeItems(p, fairyStoneList, 1)
			s.GiveItems(p, fairyStone, 1)
		}
	case strings.EqualFold(e.Name, "30608-05.htm"):
		if !checkItems(p, true) {
			htmltext = "30608-01.htm" // Avoid to continue while trade or drop mats before clicking bypass
		} else {
			s.TakeItems(p, coal, 10)
			s.TakeItems(p, charcoal, 10)
			s.TakeItems(p, gemstoneC, 1)
			s.TakeItems(p, stoneOfPurity, 1)
			s.TakeItems(p, silverNugget, 5)
			s.TakeItems(p, toadLordBackSkin, -1)
			s.TakeItems(p, deluxeFairyStoneList, 1)
			s.GiveItems(p, deluxeFairyStone, 1)
		}
	// Byron
	case strings.EqualFold(e.Name, "30711-03.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		if p.HasItem(deluxeFairyStone) {
			htmltext = "30711-04.htm"
		}
	// Mimyu
	case strings.EqualFold(e.Name, "30747-02.htm"):
		st.Set("mimyu", "1")
		s.TakeItems(p, fairyStone, 1)
	case strings.EqualFold(e.Name, "30747-04.htm"):
		st.Set("mimyu", "1")
		s.TakeItems(p, deluxeFairyStone, 1)
		s.GiveItems(p, fairyDust, 1)
	case strings.EqualFold(e.Name, "30747-07.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, juiceOfMonkshood, 1)
	case strings.EqualFold(e.Name, "30747-12.htm") && !p.HasItem(fairyDust):
		htmltext = "30747-15.htm"
		giveRandomPet(s, p, false)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30747-13.htm"):
		giveRandomPet(s, p, p.HasItem(fairyDust))
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30747-14.htm"):
		if p.HasItem(fairyDust) {
			s.TakeItems(p, fairyDust, 1)
			giveRandomPet(s, p, true)
			if s.Rnd(20) == 1 {
				s.GiveItems(p, hatchlingSoftLeather, 1)
			} else {
				htmltext = "30747-14t.htm"
				s.GiveItems(p, foodForHatchling, 20)
			}
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "30747-13.htm"
		}
	// Exarion
	case strings.EqualFold(e.Name, "30748-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, juiceOfMonkshood, 1)
		s.GiveItems(p, scaleOfDrakeExarion, 1)
	// Zwov
	case strings.EqualFold(e.Name, "30749-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, juiceOfMonkshood, 1)
		s.GiveItems(p, scaleOfDrakeZwov, 1)
	// Kalibran
	case strings.EqualFold(e.Name, "30750-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, juiceOfMonkshood, 1)
		s.GiveItems(p, scaleOfDrakeKalibran, 1)
	case strings.EqualFold(e.Name, "30750-05.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, eggOfDrakeKalibran, 19)
		s.TakeItems(p, scaleOfDrakeKalibran, 1)
	// Suzet
	case strings.EqualFold(e.Name, "30751-03.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, juiceOfMonkshood, 1)
		s.GiveItems(p, scaleOfWyvernSuzet, 1)
	// Shamhai
	case strings.EqualFold(e.Name, "30752-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, juiceOfMonkshood, 1)
		s.GiveItems(p, scaleOfWyvernShamhai, 1)
	}
	return htmltext
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
		switch e.NPC.NpcID() {
		case cooper:
			if p.Level() >= 35 {
				htmltext = "30829-01.htm"
			} else {
				htmltext = "30829-03.htm"
			}
		case mimyu:
			e.NPC.TeleportTo(mimyuLoc(q.counter.Add(1) % 3))
			return ""
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case cooper:
			htmltext = "30829-04.htm"
		case cronos:
			switch {
			case cond == 1:
				htmltext = "30610-01.htm"
			case st.GetInt("deluxestone") == 2:
				htmltext = "30610-10.htm"
			case cond == 2:
				if p.HasAtLeastOneItem(fairyStone, deluxeFairyStone) {
					if st.GetInt("deluxestone") == 1 {
						htmltext = "30610-14.htm"
					} else {
						htmltext = "30610-08.htm"
						st.SetCond(3)
						s.PlaySound(p, script.SoundMiddle)
					}
				} else {
					htmltext = "30610-07.htm"
				}
			case cond == 3:
				htmltext = "30610-09.htm"
			case cond == 4 && p.HasAtLeastOneItem(fairyStone, deluxeFairyStone):
				htmltext = "30610-11.htm"
			}
		case maria:
			switch {
			case p.HasAtLeastOneItem(fairyStone, deluxeFairyStone):
				htmltext = "30608-06.htm"
			case cond == 2:
				if p.HasItem(fairyStoneList) {
					if checkItems(p, false) {
						htmltext = "30608-02.htm"
					} else {
						htmltext = "30608-01.htm"
					}
				} else if p.HasItem(deluxeFairyStoneList) {
					if checkItems(p, true) {
						htmltext = "30608-04.htm"
					} else {
						htmltext = "30608-01.htm"
					}
				}
			}
		case byron:
			deluxestone := st.GetInt("deluxestone")
			switch {
			case deluxestone == 1:
				if p.HasItem(fairyStone) {
					htmltext = "30711-05.htm"
					st.SetCond(4)
					st.Unset("deluxestone")
					s.PlaySound(p, script.SoundMiddle)
				} else if p.HasItem(deluxeFairyStone) {
					htmltext = "30711-06.htm"
					st.SetCond(4)
					st.Unset("deluxestone")
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "30711-10.htm"
				}
			case deluxestone == 2:
				htmltext = "30711-09.htm"
			case cond == 3:
				htmltext = "30711-01.htm"
			case cond == 4:
				if p.HasItem(fairyStone) {
					htmltext = "30711-07.htm"
				} else if p.HasItem(deluxeFairyStone) {
					htmltext = "30711-08.htm"
				}
			}
		case mimyu:
			switch cond {
			case 4:
				if st.GetInt("mimyu") == 1 {
					htmltext = "30747-06.htm"
				} else if p.HasItem(fairyStone) {
					htmltext = "30747-01.htm"
				} else if p.HasItem(deluxeFairyStone) {
					htmltext = "30747-03.htm"
				}
			case 5:
				htmltext = "30747-08.htm"
			case 6:
				eggs := p.ItemCount(eggOfDrakeExarion) + p.ItemCount(eggOfDrakeZwov) + p.ItemCount(eggOfDrakeKalibran) +
					p.ItemCount(eggOfWyvernSuzet) + p.ItemCount(eggOfWyvernShamhai)
				if eggs < 20 {
					htmltext = "30747-09.htm"
				} else {
					htmltext = "30747-10.htm"
				}
			case 7:
				htmltext = "30747-11.htm"
			default:
				e.NPC.TeleportTo(mimyuLoc(q.counter.Add(1) % 3))
				return ""
			}
		case exarion:
			switch cond {
			case 5:
				htmltext = "30748-01.htm"
			case 6:
				if p.ItemCount(eggOfDrakeExarion) < 20 {
					htmltext = "30748-03.htm"
				} else {
					htmltext = "30748-04.htm"
					st.SetCond(7)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, eggOfDrakeExarion, 19)
					s.TakeItems(p, scaleOfDrakeExarion, 1)
				}
			case 7:
				htmltext = "30748-05.htm"
			}
		case zwov:
			switch cond {
			case 5:
				htmltext = "30749-01.htm"
			case 6:
				if p.ItemCount(eggOfDrakeZwov) < 20 {
					htmltext = "30749-03.htm"
				} else {
					htmltext = "30749-04.htm"
					st.SetCond(7)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, eggOfDrakeZwov, 19)
					s.TakeItems(p, scaleOfDrakeZwov, 1)
				}
			case 7:
				htmltext = "30749-05.htm"
			}
		case kalibran:
			switch cond {
			case 5:
				htmltext = "30750-01.htm"
			case 6:
				if p.ItemCount(eggOfDrakeKalibran) < 20 {
					htmltext = "30750-03.htm"
				} else {
					htmltext = "30750-04.htm"
				}
			case 7:
				htmltext = "30750-06.htm"
			}
		case suzet:
			switch cond {
			case 5:
				htmltext = "30751-01.htm"
			case 6:
				if p.ItemCount(eggOfWyvernSuzet) < 20 {
					htmltext = "30751-04.htm"
				} else {
					htmltext = "30751-05.htm"
					st.SetCond(7)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, eggOfWyvernSuzet, 19)
					s.TakeItems(p, scaleOfWyvernSuzet, 1)
				}
			case 7:
				htmltext = "30751-06.htm"
			}
		case shamhai:
			switch cond {
			case 5:
				htmltext = "30752-01.htm"
			case 6:
				if p.ItemCount(eggOfWyvernShamhai) < 20 {
					htmltext = "30752-03.htm"
				} else {
					htmltext = "30752-04.htm"
					st.SetCond(7)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, eggOfWyvernShamhai, 19)
					s.TakeItems(p, scaleOfWyvernShamhai, 1)
				}
			case 7:
				htmltext = "30752-05.htm"
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
	case 20231:
		if p.HasItem(fairyStoneList) {
			s.DropItems(p, toadLordBackSkin, 1, 10, 300000)
		} else if p.HasItem(deluxeFairyStoneList) {
			s.DropItems(p, toadLordBackSkin, 1, 20, 300000)
		}
	case 20580:
		if p.HasItem(scaleOfDrakeExarion) && !s.DropItems(p, eggOfDrakeExarion, 1, 20, 500000) {
			e.NPC.Say(script.NpcString(sayExarion))
		}
	case 20233:
		if p.HasItem(scaleOfDrakeZwov) {
			s.DropItems(p, eggOfDrakeZwov, 1, 20, 500000)
		}
	case 20551:
		if p.HasItem(scaleOfDrakeKalibran) && !s.DropItems(p, eggOfDrakeKalibran, 1, 20, 500000) {
			e.NPC.Say(script.NpcString(sayKalibran))
		}
	case 20270:
		if p.HasItem(scaleOfWyvernSuzet) && !s.DropItems(p, eggOfWyvernSuzet, 1, 20, 500000) {
			e.NPC.Say(script.NpcString(saySuzet))
		}
	case 20202:
		if p.HasItem(scaleOfWyvernShamhai) {
			s.DropItems(p, eggOfWyvernShamhai, 1, 20, 500000)
		}
	case 20589, 20590, 20591, 20592, 20593, 20594, 20595, 20596, 20597, 20598, 20599:
		if p.HasItem(deluxeFairyStone) && s.Rnd(100) < 30 {
			st.Set("deluxestone", "2")
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, deluxeFairyStone, 1)
			e.NPC.Say(script.NpcString(sayStone))
		}
	}
}

// checkItems reports whether the player holds what Maria asks for the plain
// stone, or for the deluxe one.
func checkItems(p *script.Player, isDeluxe bool) bool {
	// Conditions required for both cases.
	if p.ItemCount(coal) < 10 || p.ItemCount(charcoal) < 10 {
		return false
	}
	if isDeluxe {
		return p.ItemCount(gemstoneC) >= 1 && p.ItemCount(silverNugget) >= 5 && p.ItemCount(stoneOfPurity) >= 1 &&
			p.ItemCount(toadLordBackSkin) >= 20
	}
	return p.ItemCount(gemstoneD) >= 1 && p.ItemCount(silverNugget) >= 3 && p.ItemCount(toadLordBackSkin) >= 10
}

// giveRandomPet takes one egg and gives the flute it hatches: the egg of a
// kind decides the chance of the wind and the star flutes, the fairy dust
// raises the chance of the lesser ones.
func giveRandomPet(s *script.Script, p *script.Player, hasFairyDust bool) {
	pet := int32(dragonfluteOfTwilight)
	chance := s.Rnd(100)
	switch {
	case p.HasItem(eggOfDrakeExarion):
		s.TakeItems(p, eggOfDrakeExarion, 1)
		if hasFairyDust {
			if chance < 45 {
				pet = dragonfluteOfWind
			} else if chance < 75 {
				pet = dragonfluteOfStar
			}
		} else if chance < 50 {
			pet = dragonfluteOfWind
		} else if chance < 85 {
			pet = dragonfluteOfStar
		}
	case p.HasItem(eggOfWyvernSuzet):
		s.TakeItems(p, eggOfWyvernSuzet, 1)
		if hasFairyDust {
			if chance < 55 {
				pet = dragonfluteOfWind
			} else if chance < 85 {
				pet = dragonfluteOfStar
			}
		} else if chance < 65 {
			pet = dragonfluteOfWind
		} else if chance < 95 {
			pet = dragonfluteOfStar
		}
	case p.HasItem(eggOfDrakeKalibran):
		s.TakeItems(p, eggOfDrakeKalibran, 1)
		if hasFairyDust {
			if chance < 60 {
				pet = dragonfluteOfWind
			} else if chance < 90 {
				pet = dragonfluteOfStar
			}
		} else if chance < 70 {
			pet = dragonfluteOfWind
		} else {
			pet = dragonfluteOfStar
		}
	case p.HasItem(eggOfWyvernShamhai):
		s.TakeItems(p, eggOfWyvernShamhai, 1)
		if hasFairyDust {
			if chance < 70 {
				pet = dragonfluteOfWind
			} else {
				pet = dragonfluteOfStar
			}
		} else if chance < 85 {
			pet = dragonfluteOfWind
		} else {
			pet = dragonfluteOfStar
		}
	case p.HasItem(eggOfDrakeZwov):
		s.TakeItems(p, eggOfDrakeZwov, 1)
		if hasFairyDust {
			if chance < 90 {
				pet = dragonfluteOfWind
			} else {
				pet = dragonfluteOfStar
			}
		} else {
			pet = dragonfluteOfWind
		}
	}
	s.GiveItems(p, pet, 1)
}
