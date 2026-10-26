// Package q408 is the quest Path to an Elven Wizard: an elven mystic earns
// the Eternity Diamond by bringing Rosella a ruby, an aquamarine and an
// amethyst, each earned from one of three helpers after a hunt.
package q408

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q408_PathToAnElvenWizard"

// Quest items.
const (
	rosellaLetter      = 1218
	redDown            = 1219
	magicalPowersRuby  = 1220
	pureAquamarine     = 1221
	appetizingApple    = 1222
	goldLeaves         = 1223
	immortalLove       = 1224
	amethyst           = 1225
	nobilityAmethyst   = 1226
	fertilityPeridot   = 1229
	eternityDiamond    = 1230
	charmOfGrain       = 1272
	sapOfTheMotherTree = 1273
	luckyPotpourri     = 1274
)

// Npcs.
const (
	rosella   = 30414
	greenis   = 30157
	thalia    = 30371
	northwind = 30423
)

// Monsters.
const (
	sukarWereratLeader = 20047
	dryadElder         = 20019
	pincerSpider       = 20466
)

// The classes the quest checks.
const (
	classElvenMystic = 25
	classElvenWizard = 26
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to an Elven Wizard",
		QuestID: 408,
		Items: []int32{
			rosellaLetter, redDown, magicalPowersRuby, pureAquamarine, appetizingApple, goldLeaves, immortalLove, amethyst,
			nobilityAmethyst, fertilityPeridot, charmOfGrain, sapOfTheMotherTree, luckyPotpourri,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {rosella},
			script.EventTalked:     {rosella, greenis, thalia, northwind},
			script.EventMyDying:    {sukarWereratLeader, dryadElder, pincerSpider},
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
	case strings.EqualFold(e.Name, "30414-06.htm"):
		switch {
		case p.ClassID() != classElvenMystic:
			if p.ClassID() == classElvenWizard {
				htmltext = "30414-02a.htm"
			} else {
				htmltext = "30414-03.htm"
			}
		case p.Level() < 19:
			htmltext = "30414-04.htm"
		case p.HasItem(eternityDiamond):
			htmltext = "30414-05.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, fertilityPeridot, 1)
		}
	case strings.EqualFold(e.Name, "30414-07.htm"):
		if !p.HasItem(magicalPowersRuby) {
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, rosellaLetter, 1)
		} else {
			htmltext = "30414-10.htm"
		}
	case strings.EqualFold(e.Name, "30414-14.htm"):
		if !p.HasItem(pureAquamarine) {
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, appetizingApple, 1)
		} else {
			htmltext = "30414-13.htm"
		}
	case strings.EqualFold(e.Name, "30414-18.htm"):
		if !p.HasItem(nobilityAmethyst) {
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, immortalLove, 1)
		} else {
			htmltext = "30414-17.htm"
		}
	case strings.EqualFold(e.Name, "30157-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rosellaLetter, 1)
		s.GiveItems(p, charmOfGrain, 1)
	case strings.EqualFold(e.Name, "30371-02.htm"):
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, appetizingApple, 1)
		s.GiveItems(p, sapOfTheMotherTree, 1)
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
		htmltext = "30414-01.htm"
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case rosella:
			switch {
			case p.HasItems(magicalPowersRuby, nobilityAmethyst, pureAquamarine):
				htmltext = "30414-24.htm"
				s.TakeItems(p, fertilityPeridot, 1)
				s.TakeItems(p, magicalPowersRuby, 1)
				s.TakeItems(p, nobilityAmethyst, 1)
				s.TakeItems(p, pureAquamarine, 1)
				s.GiveItems(p, eternityDiamond, 1)
				s.RewardExpAndSp(p, 3200, 1890)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			case p.HasItem(rosellaLetter):
				htmltext = "30414-08.htm"
			case p.HasItem(charmOfGrain):
				if p.ItemCount(redDown) == 5 {
					htmltext = "30414-25.htm"
				} else {
					htmltext = "30414-09.htm"
				}
			case p.HasItem(appetizingApple):
				htmltext = "30414-15.htm"
			case p.HasItem(sapOfTheMotherTree):
				if p.ItemCount(goldLeaves) == 5 {
					htmltext = "30414-26.htm"
				} else {
					htmltext = "30414-16.htm"
				}
			case p.HasItem(immortalLove):
				htmltext = "30414-19.htm"
			case p.HasItem(luckyPotpourri):
				if p.ItemCount(amethyst) == 2 {
					htmltext = "30414-27.htm"
				} else {
					htmltext = "30414-20.htm"
				}
			default:
				htmltext = "30414-11.htm"
			}
		case greenis:
			switch {
			case p.HasItem(rosellaLetter):
				htmltext = "30157-01.htm"
			case p.ItemCount(redDown) == 5:
				htmltext = "30157-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, charmOfGrain, 1)
				s.TakeItems(p, redDown, -1)
				s.GiveItems(p, magicalPowersRuby, 1)
			case p.HasItem(charmOfGrain):
				htmltext = "30157-03.htm"
			}
		case thalia:
			switch {
			case p.HasItem(appetizingApple):
				htmltext = "30371-01.htm"
			case p.ItemCount(goldLeaves) == 5:
				htmltext = "30371-04.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, goldLeaves, -1)
				s.TakeItems(p, sapOfTheMotherTree, 1)
				s.GiveItems(p, pureAquamarine, 1)
			case p.HasItem(sapOfTheMotherTree):
				htmltext = "30371-03.htm"
			}
		case northwind:
			switch {
			case p.HasItem(immortalLove):
				htmltext = "30423-01.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, immortalLove, 1)
				s.GiveItems(p, luckyPotpourri, 1)
			case p.ItemCount(amethyst) == 2:
				htmltext = "30423-03.htm"
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, amethyst, -1)
				s.TakeItems(p, luckyPotpourri, 1)
				s.GiveItems(p, nobilityAmethyst, 1)
			case p.HasItem(luckyPotpourri):
				htmltext = "30423-02.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case dryadElder:
		if p.HasItem(sapOfTheMotherTree) {
			s.DropItems(p, goldLeaves, 1, 5, 400000)
		}
	case sukarWereratLeader:
		if p.HasItem(luckyPotpourri) {
			s.DropItems(p, amethyst, 1, 2, 400000)
		}
	case pincerSpider:
		if p.HasItem(charmOfGrain) {
			s.DropItems(p, redDown, 1, 5, 700000)
		}
	}
}
