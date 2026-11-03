// Package q415 is the quest Path to a Monk: an orc fighter proves himself
// with bare fists for Gantaki, Rosheek and Kasman or Toruku, or for Aren and
// Moira, to earn the Khavatari Totem.
package q415

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q415_PathToAMonk"

// Quest items.
const (
	pomegranate                   = 1593
	leatherPouch1                 = 1594
	leatherPouch2                 = 1595
	leatherPouch3                 = 1596
	leatherPouchFull1             = 1597
	leatherPouchFull2             = 1598
	leatherPouchFull3             = 1599
	kashaBearClaw                 = 1600
	kashaBladeSpiderTalon         = 1601
	scarletSalamanderScale        = 1602
	fierySpiritScroll             = 1603
	rosheekLetter                 = 1604
	gantakiLetterOfRecommendation = 1605
	fig                           = 1606
	leatherPouch4                 = 1607
	leatherPouchFull4             = 1608
	vukuOrcTusk                   = 1609
	ratmanFang                    = 1610
	langKlizardmanTooth           = 1611
	felimLizardmanTooth           = 1612
	ironWillScroll                = 1613
	torukuLetter                  = 1614
	khavatariTotem                = 1615
	kashaSpiderTooth              = 8545
	hornOfBaarDreVanul            = 8546
)

// Npcs.
const (
	gantaki = 30587
	rosheek = 30590
	kasman  = 30501
	toruku  = 30591
	aren    = 32056
	moira   = 31979
)

// The classes the quest checks.
const (
	classOrcFighter = 44
	classMonk       = 47
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Monk",
		QuestID: 415,
		Items: []int32{
			pomegranate, leatherPouch1, leatherPouch2, leatherPouch3, leatherPouchFull1, leatherPouchFull2,
			leatherPouchFull3, kashaBearClaw, kashaBladeSpiderTalon, scarletSalamanderScale, fierySpiritScroll,
			rosheekLetter, gantakiLetterOfRecommendation, fig, leatherPouch4, leatherPouchFull4, vukuOrcTusk,
			ratmanFang, langKlizardmanTooth, felimLizardmanTooth, ironWillScroll, torukuLetter, kashaSpiderTooth,
			hornOfBaarDreVanul,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {gantaki},
			script.EventTalked:     {gantaki, rosheek, kasman, toruku, aren, moira},
			script.EventMyDying:    {20014, 20017, 20024, 20359, 20415, 20476, 20478, 20479, 21118},
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
	case strings.EqualFold(e.Name, "30587-05.htm"):
		switch {
		case p.ClassID() != classOrcFighter:
			if p.ClassID() == classMonk {
				htmltext = "30587-02a.htm"
			} else {
				htmltext = "30587-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30587-03.htm"
		case p.HasItem(khavatariTotem):
			htmltext = "30587-04.htm"
		}
	case strings.EqualFold(e.Name, "30587-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, pomegranate, 1)
	case strings.EqualFold(e.Name, "30587-09a.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rosheekLetter, 1)
		s.GiveItems(p, gantakiLetterOfRecommendation, 1)
	case strings.EqualFold(e.Name, "30587-09b.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rosheekLetter, 1)
	case strings.EqualFold(e.Name, "32056-03.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32056-08.htm"):
		st.SetCond(20)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "31979-03.htm"):
		s.TakeItems(p, fierySpiritScroll, 1)
		s.GiveItems(p, khavatariTotem, 1)
		s.RewardExpAndSp(p, 3200, 4230)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
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
		htmltext = "30587-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case gantaki:
			switch {
			case cond == 1:
				htmltext = "30587-07.htm"
			case cond > 1 && cond < 8:
				htmltext = "30587-08.htm"
			case cond == 8:
				htmltext = "30587-09.htm"
			case cond == 9:
				htmltext = "30587-10.htm"
			case cond > 9:
				htmltext = "30587-11.htm"
			}
		case rosheek:
			switch {
			case cond == 1:
				htmltext = "30590-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, pomegranate, 1)
				s.GiveItems(p, leatherPouch1, 1)
			case cond == 2:
				htmltext = "30590-02.htm"
			case cond == 3:
				htmltext = "30590-03.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, leatherPouchFull1, 1)
				s.GiveItems(p, leatherPouch2, 1)
			case cond == 4:
				htmltext = "30590-04.htm"
			case cond == 5:
				htmltext = "30590-05.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, leatherPouchFull2, 1)
				s.GiveItems(p, leatherPouch3, 1)
			case cond == 6:
				htmltext = "30590-06.htm"
			case cond == 7:
				htmltext = "30590-07.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, leatherPouchFull3, 1)
				s.GiveItems(p, fierySpiritScroll, 1)
				s.GiveItems(p, rosheekLetter, 1)
			case cond == 8:
				htmltext = "30590-08.htm"
			case cond > 8:
				htmltext = "30590-09.htm"
			}
		case kasman:
			switch {
			case cond == 9:
				htmltext = "30501-01.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, gantakiLetterOfRecommendation, 1)
				s.GiveItems(p, fig, 1)
			case cond == 10:
				htmltext = "30501-02.htm"
			case cond == 11 || cond == 12:
				htmltext = "30501-03.htm"
			case cond == 13:
				htmltext = "30501-04.htm"
				s.TakeItems(p, fierySpiritScroll, 1)
				s.TakeItems(p, ironWillScroll, 1)
				s.TakeItems(p, torukuLetter, 1)
				s.GiveItems(p, khavatariTotem, 1)
				s.RewardExpAndSp(p, 3200, 1500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case toruku:
			switch {
			case cond == 10:
				htmltext = "30591-01.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, fig, 1)
				s.GiveItems(p, leatherPouch4, 1)
			case cond == 11:
				htmltext = "30591-02.htm"
			case cond == 12:
				htmltext = "30591-03.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, leatherPouchFull4, 1)
				s.GiveItems(p, ironWillScroll, 1)
				s.GiveItems(p, torukuLetter, 1)
			case cond == 13:
				htmltext = "30591-04.htm"
			}
		case aren:
			switch {
			case cond == 14:
				htmltext = "32056-01.htm"
			case cond == 15:
				htmltext = "32056-04.htm"
			case cond == 16:
				htmltext = "32056-05.htm"
				st.SetCond(17)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, kashaSpiderTooth, -1)
			case cond == 17:
				htmltext = "32056-06.htm"
			case cond == 18:
				htmltext = "32056-07.htm"
				st.SetCond(19)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, hornOfBaarDreVanul, -1)
			case cond == 20:
				htmltext = "32056-09.htm"
			}
		case moira:
			if cond == 20 {
				htmltext = "31979-01.htm"
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

	// The monk's path is walked bare-handed: any other weapon ends the quest.
	if weapon := script.AttackType(p); weapon != item.WeaponDualFist && weapon != item.WeaponFist {
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
		return
	}

	switch e.NPC.NpcID() {
	case 20479:
		if st.Cond() == 2 && s.DropItemsAlways(p, kashaBearClaw, 1, 5) {
			st.SetCond(3)
			s.TakeItems(p, kashaBearClaw, -1)
			s.TakeItems(p, leatherPouch1, 1)
			s.GiveItems(p, leatherPouchFull1, 1)
		}
	case 20478:
		if st.Cond() == 4 && s.DropItemsAlways(p, kashaBladeSpiderTalon, 1, 5) {
			st.SetCond(5)
			s.TakeItems(p, kashaBladeSpiderTalon, -1)
			s.TakeItems(p, leatherPouch2, 1)
			s.GiveItems(p, leatherPouchFull2, 1)
		} else if st.Cond() == 15 && s.DropItems(p, kashaSpiderTooth, 1, 6, 500000) {
			st.SetCond(16)
		}
	case 20476:
		if st.Cond() == 15 && s.DropItems(p, kashaSpiderTooth, 1, 6, 500000) {
			st.SetCond(16)
		}
	case 20415:
		if st.Cond() == 6 && s.DropItemsAlways(p, scarletSalamanderScale, 1, 5) {
			st.SetCond(7)
			s.TakeItems(p, scarletSalamanderScale, -1)
			s.TakeItems(p, leatherPouch3, 1)
			s.GiveItems(p, leatherPouchFull3, 1)
		}
	case 20014:
		if st.Cond() == 11 && s.DropItemsAlways(p, felimLizardmanTooth, 1, 3) {
			if p.ItemCount(ratmanFang) == 3 && p.ItemCount(langKlizardmanTooth) == 3 && p.ItemCount(vukuOrcTusk) == 3 {
				finishPouch(s, st, p)
			}
		}
	case 20017:
		if st.Cond() == 11 && s.DropItemsAlways(p, vukuOrcTusk, 1, 3) {
			if p.ItemCount(ratmanFang) == 3 && p.ItemCount(langKlizardmanTooth) == 3 && p.ItemCount(felimLizardmanTooth) == 3 {
				finishPouch(s, st, p)
			}
		}
	case 20024:
		if st.Cond() == 11 && s.DropItemsAlways(p, langKlizardmanTooth, 1, 3) {
			if p.ItemCount(ratmanFang) == 3 && p.ItemCount(felimLizardmanTooth) == 3 && p.ItemCount(vukuOrcTusk) == 3 {
				finishPouch(s, st, p)
			}
		}
	case 20359:
		if st.Cond() == 11 && s.DropItemsAlways(p, ratmanFang, 1, 3) {
			if p.ItemCount(langKlizardmanTooth) == 3 && p.ItemCount(felimLizardmanTooth) == 3 && p.ItemCount(vukuOrcTusk) == 3 {
				finishPouch(s, st, p)
			}
		}
	case 21118:
		if st.Cond() == 17 {
			st.SetCond(18)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, hornOfBaarDreVanul, 1)
		}
	}
}

// finishPouch trades the four gathered tusks and teeth for the full pouch.
func finishPouch(s *script.Script, st *script.QuestState, p *script.Player) {
	st.SetCond(12)
	s.TakeItems(p, vukuOrcTusk, -1)
	s.TakeItems(p, ratmanFang, -1)
	s.TakeItems(p, langKlizardmanTooth, -1)
	s.TakeItems(p, felimLizardmanTooth, -1)
	s.TakeItems(p, leatherPouch4, 1)
	s.GiveItems(p, leatherPouchFull4, 1)
}
