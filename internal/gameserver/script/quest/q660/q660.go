// Package q660 is the quest Aiding the Floran Village: Maria sends the
// character to Alex, the captain of the guard, who pays adena for the
// watching eyes, golem shards and lizardmen scales the monsters around the
// village drop, or trades them for a chance at enchant scrolls.
package q660

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q660_AidingTheFloranVillage"

// NPCs.
const (
	maria = 30608
	alex  = 30291
)

// Items.
const (
	watchingEyes   = 8074
	golemShard     = 8075
	lizardmenScale = 8076
)

// Monsters.
const (
	plainWatchman      = 21102
	rockGolem          = 21103
	lizardmenSupplier  = 21104
	lizardmenAgent     = 21105
	cursedSeer         = 21106
	lizardmenCommander = 21107
	lizardmenShaman    = 20781
)

// Rewards.
const (
	adena         = 57
	enchantWeapon = 955
	enchantArmor  = 956
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Aiding the Floran Village",
		QuestID: 660,
		Items:   []int32{watchingEyes, lizardmenScale, golemShard},
		Bind: script.Bindings{
			script.EventQuestStart: {alex, maria},
			script.EventTalked:     {alex, maria},
			script.EventMyDying: {
				lizardmenShaman, plainWatchman, rockGolem, lizardmenSupplier, lizardmenAgent,
				cursedSeer, lizardmenCommander,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays for the items held, trades them for a
// random reward, or ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30608-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30291-02.htm"):
		if e.Player.Level() < 30 {
			htmltext = "30291-02a.htm"
		} else {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(2)
			s.PlaySound(e.Player, script.SoundAccept)
		}
	case strings.EqualFold(e.Name, "30291-05.htm"):
		count := heldItems(e.Player)
		if count == 0 {
			htmltext = "30291-05a.htm"
		} else {
			s.TakeItems(e.Player, golemShard, -1)
			s.TakeItems(e.Player, lizardmenScale, -1)
			s.TakeItems(e.Player, watchingEyes, -1)
			s.RewardItems(e.Player, adena, payment(count))
		}
	case strings.EqualFold(e.Name, "30291-06.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30291-11.htm"):
		if !verifyAndRemoveItems(s, e.Player, 100) {
			htmltext = "30291-11a.htm"
		} else if s.Rnd(10) < 8 {
			s.RewardItems(e.Player, adena, 1000)
		} else {
			s.RewardItems(e.Player, adena, 13000)
			s.RewardItems(e.Player, enchantArmor, 1)
		}
	case strings.EqualFold(e.Name, "30291-12.htm"):
		if !verifyAndRemoveItems(s, e.Player, 200) {
			htmltext = "30291-12a.htm"
		} else {
			luck := s.Rnd(15)
			switch {
			case luck < 8:
				s.RewardItems(e.Player, adena, 2000)
			case luck < 12:
				s.RewardItems(e.Player, adena, 20000)
				s.RewardItems(e.Player, enchantArmor, 1)
			default:
				s.RewardItems(e.Player, enchantWeapon, 1)
			}
		}
	case strings.EqualFold(e.Name, "30291-13.htm"):
		if !verifyAndRemoveItems(s, e.Player, 500) {
			htmltext = "30291-13a.htm"
		} else if s.Rnd(10) < 8 {
			s.RewardItems(e.Player, adena, 5000)
		} else {
			s.RewardItems(e.Player, adena, 45000)
			s.RewardItems(e.Player, enchantWeapon, 1)
		}
	case strings.EqualFold(e.Name, "30291-17.htm"):
		count := heldItems(e.Player)
		if count != 0 {
			htmltext = "30291-17a.htm"
			s.TakeItems(e.Player, watchingEyes, -1)
			s.TakeItems(e.Player, lizardmenScale, -1)
			s.TakeItems(e.Player, golemShard, -1)
			s.RewardItems(e.Player, adena, payment(count))
		}
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// heldItems returns how many eyes, scales and shards p holds.
func heldItems(p *script.Player) int32 {
	return p.ItemCount(watchingEyes) + p.ItemCount(lizardmenScale) + p.ItemCount(golemShard)
}

// payment returns the adena for count items, with a bonus from 45.
func payment(count int32) int32 {
	if count >= 45 {
		return count*100 + 9000
	}
	return count * 100
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		switch e.NPC.NpcID() {
		case maria:
			if e.Player.Level() < 30 {
				htmltext = "30608-01.htm"
			} else {
				htmltext = "30608-02.htm"
			}
		case alex:
			htmltext = "30291-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case maria:
			htmltext = "30608-06.htm"
		case alex:
			switch st.Cond() {
			case 1:
				htmltext = "30291-03.htm"
				st.SetCond(2)
				s.PlaySound(e.Player, script.SoundMiddle)
			case 2:
				if e.Player.HasAtLeastOneItem(watchingEyes, lizardmenScale, golemShard) {
					htmltext = "30291-04.htm"
				} else {
					htmltext = "30291-05a.htm"
				}
			}
		}
	}
	return htmltext
}

// onMyDying drops an item to one party member at random among those at cond 2
// near the monster.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case plainWatchman, cursedSeer:
		s.DropItems(st.Player(), watchingEyes, 1, 0, 790000)
	case rockGolem:
		s.DropItems(st.Player(), golemShard, 1, 0, 750000)
	case lizardmenShaman, lizardmenSupplier, lizardmenAgent, lizardmenCommander:
		s.DropItems(st.Player(), lizardmenScale, 1, 0, 670000)
	}
}

// verifyAndRemoveItems takes n of the held items, the eyes first, then the
// scales, then the shards; it takes nothing and returns false when fewer than
// n are held.
func verifyAndRemoveItems(s *script.Script, p *script.Player, n int32) bool {
	eyes := p.ItemCount(watchingEyes)
	scale := p.ItemCount(lizardmenScale)
	shard := p.ItemCount(golemShard)
	if eyes+scale+shard < n {
		return false
	}
	if eyes >= n {
		s.TakeItems(p, watchingEyes, n)
		return true
	}
	cur := n - eyes
	s.TakeItems(p, watchingEyes, -1)
	if scale >= cur {
		s.TakeItems(p, lizardmenScale, cur)
		return true
	}
	cur -= scale
	s.TakeItems(p, lizardmenScale, -1)
	s.TakeItems(p, golemShard, cur)
	return true
}
