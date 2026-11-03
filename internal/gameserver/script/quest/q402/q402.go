// Package q402 is the quest Path to a Human Knight: a human fighter earns
// the Sword of Ritual from Sir Klaus Vasper by bringing him the coins of
// the lords, each earned by a hunt for a mark.
package q402

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q402_PathToAHumanKnight"

// Quest items.
const (
	swordOfRitual       = 1161
	coinOfLords1        = 1162
	coinOfLords2        = 1163
	coinOfLords3        = 1164
	coinOfLords4        = 1165
	coinOfLords5        = 1166
	coinOfLords6        = 1167
	gludioGuardMark1    = 1168
	bugbearNecklace     = 1169
	einhasadChurchMark1 = 1170
	einhasadCrucifix    = 1171
	gludioGuardMark2    = 1172
	spiderLeg           = 1173
	einhasadChurchMark2 = 1174
	lizardmanTotem      = 1175
	gludioGuardMark3    = 1176
	giantSpiderHusk     = 1177
	einhasadChurchMark3 = 1178
	horribleSkull       = 1179
	markOfEsquire       = 1271
)

// Npcs.
const (
	sirKlausVasper     = 30417
	bathis             = 30332
	raymond            = 30289
	bezique            = 30379
	levian             = 30037
	gilbert            = 30039
	biotin             = 30031
	sirAaronTanford    = 30653
	sirCollinWindawood = 30311
)

// The classes the quest checks.
const (
	classHumanFighter = 0x00
	classKnight       = 0x04
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Human Knight",
		QuestID: 402,
		Items: []int32{
			markOfEsquire, coinOfLords1, coinOfLords2, coinOfLords3, coinOfLords4, coinOfLords5, coinOfLords6,
			gludioGuardMark1, bugbearNecklace, einhasadChurchMark1, einhasadCrucifix, gludioGuardMark2, spiderLeg,
			einhasadChurchMark2, lizardmanTotem, gludioGuardMark3, giantSpiderHusk, einhasadChurchMark3,
			lizardmanTotem, gludioGuardMark3, giantSpiderHusk, einhasadChurchMark3, horribleSkull,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {sirKlausVasper},
			script.EventTalked: {
				sirKlausVasper, bathis, raymond, bezique, levian, gilbert, biotin, sirAaronTanford, sirCollinWindawood,
			},
			script.EventMyDying: {20775, 27024, 20038, 20043, 20050, 20030, 20027, 20024, 20103, 20106, 20108, 20404},
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
	case strings.EqualFold(e.Name, "30417-05.htm"):
		switch {
		case p.ClassID() != classHumanFighter:
			if p.ClassID() == classKnight {
				htmltext = "30417-02a.htm"
			} else {
				htmltext = "30417-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30417-02.htm"
		case p.HasItem(swordOfRitual):
			htmltext = "30417-04.htm"
		}
	case strings.EqualFold(e.Name, "30417-08.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, markOfEsquire, 1)
	case strings.EqualFold(e.Name, "30332-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gludioGuardMark1, 1)
	case strings.EqualFold(e.Name, "30289-03.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, einhasadChurchMark1, 1)
	case strings.EqualFold(e.Name, "30379-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gludioGuardMark2, 1)
	case strings.EqualFold(e.Name, "30037-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, einhasadChurchMark2, 1)
	case strings.EqualFold(e.Name, "30039-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gludioGuardMark3, 1)
	case strings.EqualFold(e.Name, "30031-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, einhasadChurchMark3, 1)
	case strings.EqualFold(e.Name, "30417-13.htm") || strings.EqualFold(e.Name, "30417-14.htm"):
		coinCount := p.ItemCount(coinOfLords1) + p.ItemCount(coinOfLords2) + p.ItemCount(coinOfLords3) + p.ItemCount(coinOfLords4) + p.ItemCount(coinOfLords5) + p.ItemCount(coinOfLords6)

		s.TakeItems(p, coinOfLords1, -1)
		s.TakeItems(p, coinOfLords2, -1)
		s.TakeItems(p, coinOfLords3, -1)
		s.TakeItems(p, coinOfLords4, -1)
		s.TakeItems(p, coinOfLords5, -1)
		s.TakeItems(p, coinOfLords6, -1)
		s.TakeItems(p, markOfEsquire, 1)
		s.GiveItems(p, swordOfRitual, 1)
		s.RewardExpAndSp(p, 3200, 1500+(1920*(coinCount-3)))
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
		htmltext = "30417-01.htm"
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	}
	return htmltext
}

// talkStarted is the page npcID gives a started quest, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	switch npcID {
	case sirKlausVasper:
		coins := p.ItemCount(coinOfLords1) + p.ItemCount(coinOfLords2) + p.ItemCount(coinOfLords3) + p.ItemCount(coinOfLords4) + p.ItemCount(coinOfLords5) + p.ItemCount(coinOfLords6)
		switch {
		case coins < 3:
			htmltext = "30417-09.htm"
		case coins == 3:
			htmltext = "30417-10.htm"
		case coins > 3 && coins < 6:
			htmltext = "30417-11.htm"
		case coins == 6:
			htmltext = "30417-12.htm"
			s.TakeItems(p, coinOfLords1, -1)
			s.TakeItems(p, coinOfLords2, -1)
			s.TakeItems(p, coinOfLords3, -1)
			s.TakeItems(p, coinOfLords4, -1)
			s.TakeItems(p, coinOfLords5, -1)
			s.TakeItems(p, coinOfLords6, -1)
			s.TakeItems(p, markOfEsquire, 1)
			s.GiveItems(p, swordOfRitual, 1)
			s.RewardExpAndSp(p, 3200, 7260)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	case bathis:
		switch {
		case p.HasItem(coinOfLords1):
			htmltext = "30332-05.htm"
		case p.HasItem(gludioGuardMark1):
			if p.ItemCount(bugbearNecklace) < 10 {
				htmltext = "30332-03.htm"
			} else {
				htmltext = "30332-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bugbearNecklace, -1)
				s.TakeItems(p, gludioGuardMark1, 1)
				s.GiveItems(p, coinOfLords1, 1)
			}
		default:
			htmltext = "30332-01.htm"
		}
	case raymond:
		switch {
		case p.HasItem(coinOfLords2):
			htmltext = "30289-06.htm"
		case p.HasItem(einhasadChurchMark1):
			if p.ItemCount(einhasadCrucifix) < 12 {
				htmltext = "30289-04.htm"
			} else {
				htmltext = "30289-05.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, einhasadCrucifix, -1)
				s.TakeItems(p, einhasadChurchMark1, 1)
				s.GiveItems(p, coinOfLords2, 1)
			}
		default:
			htmltext = "30289-01.htm"
		}
	case bezique:
		switch {
		case p.HasItem(coinOfLords3):
			htmltext = "30379-05.htm"
		case p.HasItem(gludioGuardMark2):
			if p.ItemCount(spiderLeg) < 20 {
				htmltext = "30379-03.htm"
			} else {
				htmltext = "30379-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, spiderLeg, -1)
				s.TakeItems(p, gludioGuardMark2, 1)
				s.GiveItems(p, coinOfLords3, 1)
			}
		default:
			htmltext = "30379-01.htm"
		}
	case levian:
		switch {
		case p.HasItem(coinOfLords4):
			htmltext = "30037-05.htm"
		case p.HasItem(einhasadChurchMark2):
			if p.ItemCount(lizardmanTotem) < 20 {
				htmltext = "30037-03.htm"
			} else {
				htmltext = "30037-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, lizardmanTotem, -1)
				s.TakeItems(p, einhasadChurchMark2, 1)
				s.GiveItems(p, coinOfLords4, 1)
			}
		default:
			htmltext = "30037-01.htm"
		}
	case gilbert:
		switch {
		case p.HasItem(coinOfLords5):
			htmltext = "30039-05.htm"
		case p.HasItem(gludioGuardMark3):
			if p.ItemCount(giantSpiderHusk) < 20 {
				htmltext = "30039-03.htm"
			} else {
				htmltext = "30039-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, giantSpiderHusk, -1)
				s.TakeItems(p, gludioGuardMark3, 1)
				s.GiveItems(p, coinOfLords5, 1)
			}
		default:
			htmltext = "30039-01.htm"
		}
	case biotin:
		switch {
		case p.HasItem(coinOfLords6):
			htmltext = "30031-05.htm"
		case p.HasItem(einhasadChurchMark3):
			if p.ItemCount(horribleSkull) < 10 {
				htmltext = "30031-03.htm"
			} else {
				htmltext = "30031-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, horribleSkull, -1)
				s.TakeItems(p, einhasadChurchMark3, 1)
				s.GiveItems(p, coinOfLords6, 1)
			}
		default:
			htmltext = "30031-01.htm"
		}
	case sirAaronTanford:
		htmltext = "30653-01.htm"
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
	case 20775: // Bugbear Raider
		if p.HasItem(gludioGuardMark1) {
			s.DropItemsAlways(p, bugbearNecklace, 1, 10)
		}
	case 27024: // Undead Priest
		if p.HasItem(einhasadChurchMark1) {
			s.DropItems(p, einhasadCrucifix, 1, 12, 500000)
		}
	case 20038, 20043, 20050: // Poison Spider, Arachnid Tracker, Arachnid Predator
		if p.HasItem(gludioGuardMark2) {
			s.DropItemsAlways(p, spiderLeg, 1, 20)
		}
	case 20024, 20027, 20030: // Langk Lizardman Warrior, Langk Lizardman Scout, Langk Lizardman
		if p.HasItem(einhasadChurchMark2) {
			s.DropItems(p, lizardmanTotem, 1, 20, 500000)
		}
	case 20103, 20106, 20108: // Giant Spider, Talon Spider, Blade Spider
		if p.HasItem(gludioGuardMark3) {
			s.DropItems(p, giantSpiderHusk, 1, 20, 400000)
		}
	case 20404: // Silent Horror
		if p.HasItem(einhasadChurchMark3) {
			s.DropItems(p, horribleSkull, 1, 10, 400000)
		}
	}
}
