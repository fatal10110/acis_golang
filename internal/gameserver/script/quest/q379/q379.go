// Package q379 is the quest Fantasy Wine: Harlan trades the leaves and
// stones of the Enku Orcs for a wine of random strength.
package q379

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q379_FantasyWine"

// Npcs.
const harlan = 30074

// Monsters.
const (
	enkuChampion = 20291
	enkuShaman   = 20292
)

// Quest items.
const (
	leaf  = 5893
	stone = 5894
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Fantasy Wine",
		QuestID: 379,
		Items:   []int32{leaf, stone},
		Bind: script.Bindings{
			script.EventQuestStart: {harlan},
			script.EventTalked:     {harlan},
			script.EventMyDying:    {enkuChampion, enkuShaman},
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
	case strings.EqualFold(e.Name, "30074-3.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30074-6.htm"):
		s.TakeItems(p, leaf, 80)
		s.TakeItems(p, stone, 100)
		rand := s.Rnd(10)
		if rand < 3 {
			htmltext = "30074-6.htm"
			s.GiveItems(p, 5956, 1)
		} else if rand < 9 {
			htmltext = "30074-7.htm"
			s.GiveItems(p, 5957, 1)
		} else {
			htmltext = "30074-8.htm"
			s.GiveItems(p, 5958, 1)
		}
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30074-2a.htm"):
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
		if p.Level() < 20 {
			htmltext = "30074-0a.htm"
		} else {
			htmltext = "30074-0.htm"
		}
	case questlog.StatusStarted:
		leaves := p.ItemCount(leaf)
		stones := p.ItemCount(stone)
		switch {
		case leaves == 80 && stones == 100:
			htmltext = "30074-5.htm"
		case leaves == 80:
			htmltext = "30074-4a.htm"
		case stones == 100:
			htmltext = "30074-4b.htm"
		default:
			htmltext = "30074-4.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	player := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(player, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if e.NPC.NpcID() == enkuChampion {
		if s.DropItemsAlways(player, leaf, 1, 80) && player.ItemCount(stone) >= 100 {
			st.SetCond(2)
		}
	} else if s.DropItemsAlways(player, stone, 1, 100) && player.ItemCount(leaf) >= 80 {
		st.SetCond(2)
	}
}
