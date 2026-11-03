// Package q416 is the quest Path to an Orc Shaman: an orc mystic gathers
// fiery charms and totem blood for Tataru Zu Hestui, Umos and Moira to earn
// the Mask of Medium.
package q416

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q416_PathToAnOrcShaman"

// Quest items.
const (
	fireCharm            = 1616
	kashaBearPelt        = 1617
	kashaBladeSpiderHusk = 1618
	fieryEgg1            = 1619
	hestuiMask           = 1620
	fieryEgg2            = 1621
	totemSpiritClaw      = 1622
	tataruLetter         = 1623
	flameCharm           = 1624
	grizzlyBlood         = 1625
	bloodCauldron        = 1626
	spiritNet            = 1627
	boundDurkaSpirit     = 1628
	durkaParasite        = 1629
	totemSpiritBlood     = 1630
	maskOfMedium         = 1631
)

// Npcs.
const (
	tataruZuHestui      = 30585
	umos                = 30502
	hestuiTotemSpirit   = 30592
	dudaMaraTotemSpirit = 30593
	moira               = 31979
	totemSpiritOfGandi  = 32057
	deadLeopardCarcass  = 32090
	venomousSpider      = 20038
	arachnidTracker     = 20043
	grizzlyBear         = 20335
	scarletSalamander   = 20415
	kashaBladeSpider    = 20478
	kashaBear           = 20479
	durkaSpirit         = 27056
	blackLeopard        = 27319
)

// The classes the quest checks.
const (
	classOrcMystic = 49
	classOrcShaman = 50
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path To An Orc Shaman",
		QuestID: 416,
		Items: []int32{
			fireCharm, kashaBearPelt, kashaBladeSpiderHusk, fieryEgg1, hestuiMask, fieryEgg2, totemSpiritClaw,
			tataruLetter, flameCharm, grizzlyBlood, bloodCauldron, spiritNet, boundDurkaSpirit, durkaParasite,
			totemSpiritBlood,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {tataruZuHestui},
			script.EventTalked: {
				tataruZuHestui, umos, hestuiTotemSpirit, dudaMaraTotemSpirit, moira, totemSpiritOfGandi,
				deadLeopardCarcass,
			},
			script.EventMyDying: {
				venomousSpider, arachnidTracker, grizzlyBear, scarletSalamander, kashaBladeSpider, kashaBear,
				durkaSpirit, blackLeopard,
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
	// Tataru Zu Hestui
	case strings.EqualFold(e.Name, "30585-05.htm"):
		switch {
		case p.ClassID() != classOrcMystic:
			if p.ClassID() == classOrcShaman {
				htmltext = "30585-02a.htm"
			} else {
				htmltext = "30585-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30585-03.htm"
		case p.HasItem(maskOfMedium):
			htmltext = "30585-04.htm"
		}
	case strings.EqualFold(e.Name, "30585-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, fireCharm, 1)
	case strings.EqualFold(e.Name, "30585-11b.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, totemSpiritClaw, 1)
		s.GiveItems(p, tataruLetter, 1)
	case strings.EqualFold(e.Name, "30585-11c.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, totemSpiritClaw, 1)
	// Hestui Totem Spirit
	case strings.EqualFold(e.Name, "30592-03.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hestuiMask, 1)
		s.TakeItems(p, fieryEgg2, 1)
		s.GiveItems(p, totemSpiritClaw, 1)
	// Duda Mara Totem Spirit
	case strings.EqualFold(e.Name, "30593-03.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bloodCauldron, 1)
		s.GiveItems(p, spiritNet, 1)
	// Totem Spirit of Gandi
	case strings.EqualFold(e.Name, "32057-02.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "32057-05.htm"):
		st.SetCond(21)
		s.PlaySound(p, script.SoundMiddle)
	// Dead Leopard Carcass
	case strings.EqualFold(e.Name, "32090-04.htm"):
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
	// Umos
	case strings.EqualFold(e.Name, "30502-07.htm"):
		s.TakeItems(p, totemSpiritBlood, -1)
		s.GiveItems(p, maskOfMedium, 1)
		s.RewardExpAndSp(p, 3200, 2600)
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
		htmltext = "30585-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case tataruZuHestui:
			switch {
			case cond == 1:
				htmltext = "30585-07.htm"
			case cond == 2:
				htmltext = "30585-08.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, fieryEgg1, 1)
				s.TakeItems(p, fireCharm, 1)
				s.TakeItems(p, kashaBearPelt, 1)
				s.TakeItems(p, kashaBladeSpiderHusk, 1)
				s.GiveItems(p, fieryEgg2, 1)
				s.GiveItems(p, hestuiMask, 1)
			case cond == 3:
				htmltext = "30585-09.htm"
			case cond == 4:
				htmltext = "30585-10.htm"
			case cond == 5:
				htmltext = "30585-12.htm"
			case cond > 5 && cond < 12:
				htmltext = "30585-13.htm"
			case cond == 12:
				htmltext = "30585-11c.htm"
			}
		case hestuiTotemSpirit:
			switch {
			case cond == 3:
				htmltext = "30592-01.htm"
			case cond == 4:
				htmltext = "30592-04.htm"
			case cond > 4 && cond < 12:
				htmltext = "30592-05.htm"
			}
		case umos:
			switch {
			case cond == 5:
				htmltext = "30502-01.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, tataruLetter, 1)
				s.GiveItems(p, flameCharm, 1)
			case cond == 6:
				htmltext = "30502-02.htm"
			case cond == 7:
				htmltext = "30502-03.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, flameCharm, 1)
				s.TakeItems(p, grizzlyBlood, 3)
				s.GiveItems(p, bloodCauldron, 1)
			case cond == 8:
				htmltext = "30502-04.htm"
			case cond == 9 || cond == 10:
				htmltext = "30502-05.htm"
			case cond == 11:
				htmltext = "30502-06.htm"
			}
		case moira:
			switch {
			case cond == 12:
				htmltext = "31979-01.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
			case cond > 12 && cond < 21:
				htmltext = "31979-02.htm"
			case cond == 21:
				htmltext = "31979-03.htm"
				s.GiveItems(p, maskOfMedium, 1)
				s.RewardExpAndSp(p, 3200, 3250)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case totemSpiritOfGandi:
			switch {
			case cond == 13:
				htmltext = "32057-01.htm"
			case cond > 13 && cond < 20:
				htmltext = "32057-03.htm"
			case cond == 20:
				htmltext = "32057-04.htm"
			}
		case dudaMaraTotemSpirit:
			switch {
			case cond == 8:
				htmltext = "30593-01.htm"
			case cond == 9:
				htmltext = "30593-04.htm"
			case cond == 10:
				htmltext = "30593-05.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, boundDurkaSpirit, 1)
				s.GiveItems(p, totemSpiritBlood, 1)
			case cond == 11:
				htmltext = "30593-06.htm"
			}
		case deadLeopardCarcass:
			switch {
			case cond == 14:
				htmltext = "32090-01a.htm"
			case cond == 15:
				htmltext = "32090-01.htm"
				st.SetCond(16)
				s.PlaySound(p, script.SoundMiddle)
			case cond == 16:
				htmltext = "32090-01b.htm"
			case cond == 17:
				htmltext = "32090-02.htm"
			case cond == 18:
				htmltext = "32090-05.htm"
			case cond == 19:
				htmltext = "32090-06.htm"
				st.SetCond(20)
				s.PlaySound(p, script.SoundMiddle)
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

	cond := st.Cond()
	switch e.NPC.NpcID() {
	case kashaBear:
		if cond == 1 && !p.HasItem(kashaBearPelt) {
			s.GiveItems(p, kashaBearPelt, 1)
			if p.HasItems(fieryEgg1, kashaBladeSpiderHusk) {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case kashaBladeSpider:
		if cond == 1 && !p.HasItem(kashaBladeSpiderHusk) {
			s.GiveItems(p, kashaBladeSpiderHusk, 1)
			if p.HasItems(kashaBearPelt, fieryEgg1) {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case scarletSalamander:
		if cond == 1 && !p.HasItem(fieryEgg1) {
			s.GiveItems(p, fieryEgg1, 1)
			if p.HasItems(kashaBearPelt, kashaBladeSpiderHusk) {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case grizzlyBear:
		if cond == 6 && s.DropItemsAlways(p, grizzlyBlood, 1, 3) {
			st.SetCond(7)
		}
	case venomousSpider, arachnidTracker:
		if cond == 9 {
			count := p.ItemCount(durkaParasite)
			rnd := s.Rnd(10)
			if (count == 5 && rnd < 1) || ((count == 6 || count == 7) && rnd < 2) || count >= 8 {
				s.PlaySound(p, script.SoundBeforeBattle)
				s.TakeItems(p, durkaParasite, -1)
				s.Spawner().AddSpawn(durkaSpirit, e.NPC, false, 120000*time.Millisecond)
			} else {
				s.DropItemsAlways(p, durkaParasite, 1, 0)
			}
		}
	case durkaSpirit:
		if cond == 9 {
			st.SetCond(10)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, durkaParasite, -1)
			s.TakeItems(p, spiritNet, 1)
			s.GiveItems(p, boundDurkaSpirit, 1)
		}
	case blackLeopard:
		switch cond {
		case 14:
			if st.GetInt("leopard") > 0 {
				st.SetCond(15)
				s.PlaySound(p, script.SoundMiddle)

				if s.Rnd(3) < 2 {
					e.NPC.Say("My dear friend of " + p.Name() + ", who has gone on ahead of me!")
				}
			} else {
				st.Set("leopard", strconv.Itoa(1))
			}
		case 16:
			st.SetCond(17)
			s.PlaySound(p, script.SoundMiddle)

			if s.Rnd(3) < 2 {
				e.NPC.Say("Listen to Tejakar Gandi, young Oroka! The spirit of the slain leopard is calling you, " + p.Name() + "!")
			}
		case 18:
			st.SetCond(19)
			s.PlaySound(p, script.SoundMiddle)
		}
	}
}
