// Package q171 is the quest Acts of Evil: Guard Alvah sends a hunter after
// the Turek orcs' blade molds, the Tumran bugbears' ranger reports and the
// Ol Mahum's weapon trade, ending with thirty Ol Mahum captains' heads for
// Turek chief Burai.
package q171

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q171_ActsOfEvil"

// Items.
const (
	bladeMold           = 4239
	tyraBill            = 4240
	rangerReport1       = 4241
	rangerReport2       = 4242
	rangerReport3       = 4243
	rangerReport4       = 4244
	weaponTradeContract = 4245
	attackDirectives    = 4246
	certificate         = 4247
	cargoBox            = 4248
	olMahumHead         = 4249
)

// NPCs.
const (
	alvah               = 30381
	arodin              = 30207
	tyra                = 30420
	rolento             = 30437
	neti                = 30425
	burai               = 30617
	olMahumSupportTroop = 27190
)

// sayTroopLeaves is the line of a support troop that leaves alive.
const sayTroopLeaves = 17151

// chances returns the blade mold drop chance of each Turek orc, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		20496: 530000,
		20497: 550000,
		20498: 510000,
		20499: 500000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Acts of Evil",
		QuestID: 171,
		Items: []int32{
			bladeMold, tyraBill, rangerReport1, rangerReport2, rangerReport3, rangerReport4,
			weaponTradeContract, attackDirectives, certificate, cargoBox, olMahumHead,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {alvah},
			script.EventTalked:     {alvah, arodin, tyra, rolento, neti, burai},
			script.EventDecayed:    {olMahumSupportTroop},
			script.EventMyDying:    {20496, 20497, 20498, 20499, 20062, 20064, 20066, 20438},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnDecayed: onDecayed, OnMyDying: onMyDying},
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
	case strings.EqualFold(e.Name, "30381-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30207-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30381-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30381-07.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, weaponTradeContract, 1)
	case strings.EqualFold(e.Name, "30437-03.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, cargoBox, 1)
		s.GiveItems(p, certificate, 1)
	case strings.EqualFold(e.Name, "30617-04.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, attackDirectives, 1)
		s.TakeItems(p, cargoBox, 1)
		s.TakeItems(p, certificate, 1)
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
		if p.Level() < 27 {
			htmltext = "30381-01a.htm"
		} else {
			htmltext = "30381-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case alvah:
			switch {
			case cond < 4:
				htmltext = "30381-02a.htm"
			case cond == 4:
				htmltext = "30381-03.htm"
			case cond == 5:
				if p.HasItems(rangerReport1, rangerReport2, rangerReport3, rangerReport4) {
					htmltext = "30381-05.htm"
					st.SetCond(6)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, rangerReport1, 1)
					s.TakeItems(p, rangerReport2, 1)
					s.TakeItems(p, rangerReport3, 1)
					s.TakeItems(p, rangerReport4, 1)
				} else {
					htmltext = "30381-04a.htm"
				}
			case cond == 6:
				if p.HasItems(weaponTradeContract, attackDirectives) {
					htmltext = "30381-06.htm"
				} else {
					htmltext = "30381-05a.htm"
				}
			case cond > 6 && cond < 11:
				htmltext = "30381-07a.htm"
			case cond == 11:
				htmltext = "30381-08.htm"
				s.RewardItems(p, 57, 90000)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case arodin:
			switch {
			case cond == 1:
				htmltext = "30207-01.htm"
			case cond == 2:
				htmltext = "30207-01a.htm"
			case cond == 3:
				if p.HasItem(tyraBill) {
					htmltext = "30207-03.htm"
					st.SetCond(4)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, tyraBill, 1)
				} else {
					htmltext = "30207-01a.htm"
				}
			case cond > 3:
				htmltext = "30207-03a.htm"
			}
		case tyra:
			switch {
			case cond == 2:
				if p.ItemCount(bladeMold) >= 20 {
					htmltext = "30420-01.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, bladeMold, -1)
					s.GiveItems(p, tyraBill, 1)
				} else {
					htmltext = "30420-01b.htm"
				}
			case cond == 3:
				htmltext = "30420-01a.htm"
			case cond > 3:
				htmltext = "30420-02.htm"
			}
		case neti:
			switch {
			case cond == 7:
				htmltext = "30425-01.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
			case cond > 7:
				htmltext = "30425-02.htm"
			}
		case rolento:
			switch {
			case cond == 8:
				htmltext = "30437-01.htm"
			case cond > 8:
				htmltext = "30437-03a.htm"
			}
		case burai:
			switch {
			case cond == 9 && p.HasItems(certificate, cargoBox, attackDirectives):
				htmltext = "30617-01.htm"
			case cond == 10:
				if p.ItemCount(olMahumHead) >= 30 {
					htmltext = "30617-05.htm"
					st.SetCond(11)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, olMahumHead, -1)
					s.RewardItems(p, 57, 8000)
				} else {
					htmltext = "30617-04a.htm"
				}
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// onDecayed has a support troop that leaves alive say so. An NPC is dead
// by the time its decayed hook runs, so the line is never said.
func onDecayed(_ *script.Script, e script.Decayed) {
	if !e.NPC.Dead() {
		e.NPC.Say(script.NpcString(sayTroopLeaves))
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch npcID {
	case 20496, 20497, 20498, 20499:
		if st.Cond() == 2 && !s.DropItems(p, bladeMold, 1, 20, chances()[npcID]) {
			count := p.ItemCount(bladeMold)
			if count == 5 || (count >= 10 && s.Rnd(100) < 25) {
				troop := s.Spawner().AddSpawn(olMahumSupportTroop, e.NPC, false, 200000*time.Millisecond)
				if troop != nil {
					troop.AddAttackDesire(p, 2000)
				}
			}
		}
	case 20062, 20064:
		if st.Cond() == 5 {
			if !p.HasItem(rangerReport1) {
				s.GiveItems(p, rangerReport1, 1)
				s.PlaySound(p, script.SoundItemGet)
			} else if s.Rnd(100) < 20 {
				switch {
				case !p.HasItem(rangerReport2):
					s.GiveItems(p, rangerReport2, 1)
					s.PlaySound(p, script.SoundItemGet)
				case !p.HasItem(rangerReport3):
					s.GiveItems(p, rangerReport3, 1)
					s.PlaySound(p, script.SoundItemGet)
				case !p.HasItem(rangerReport4):
					s.GiveItems(p, rangerReport4, 1)
					s.PlaySound(p, script.SoundItemGet)
				}
			}
		}
	case 20438:
		if st.Cond() == 6 && s.Rnd(100) < 10 && !p.HasItems(weaponTradeContract, attackDirectives) {
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, weaponTradeContract, 1)
			s.GiveItems(p, attackDirectives, 1)
		}
	case 20066:
		if st.Cond() == 10 {
			s.DropItems(p, olMahumHead, 1, 30, 500000)
		}
	}
}
