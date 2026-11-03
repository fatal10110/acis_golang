// Package q356 is the quest Dig Up the Sea of Spores: Magister Gauen wants
// fifty herb spores of the rotting trees and fifty carnivore spores of the
// spore zombies.
package q356

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q356_DigUpTheSeaOfSpores"

// gauen is the quest giver.
const gauen = 30717

// Items.
const (
	herbSpore = 5866
	carnSpore = 5865
)

// Monsters.
const (
	rottingTree = 20558
	sporeZombie = 20562
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dig Up the Sea of Spores!",
		QuestID: 356,
		Items:   []int32{herbSpore, carnSpore},
		Bind: script.Bindings{
			script.EventQuestStart: {gauen},
			script.EventTalked:     {gauen},
			script.EventMyDying:    {rottingTree, sporeZombie},
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
	case strings.EqualFold(e.Name, "30717-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30717-17.htm"):
		s.TakeItems(p, herbSpore, -1)
		s.TakeItems(p, carnSpore, -1)
		s.RewardItems(p, 57, 20950)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30717-14.htm"):
		s.TakeItems(p, herbSpore, -1)
		s.TakeItems(p, carnSpore, -1)
		s.RewardExpAndSp(p, 35000, 2600)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30717-12.htm"):
		s.TakeItems(p, herbSpore, -1)
		s.RewardExpAndSp(p, 24500, 0)
	case strings.EqualFold(e.Name, "30717-13.htm"):
		s.TakeItems(p, carnSpore, -1)
		s.RewardExpAndSp(p, 0, 1820)
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
		if p.Level() < 43 {
			htmltext = "30717-01.htm"
		} else {
			htmltext = "30717-02.htm"
		}
	case questlog.StatusStarted:
		switch cond := st.Cond(); cond {
		case 1:
			htmltext = "30717-07.htm"
		case 2:
			switch {
			case p.ItemCount(herbSpore) >= 50:
				htmltext = "30717-08.htm"
			case p.ItemCount(carnSpore) >= 50:
				htmltext = "30717-09.htm"
			default:
				htmltext = "30717-07.htm"
			}
		case 3:
			htmltext = "30717-10.htm"
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
	if cond >= 3 {
		return
	}
	next := int32(2)
	if cond == 2 {
		next = 3
	}
	switch e.NPC.NpcID() {
	case rottingTree:
		if s.DropItems(p, herbSpore, 1, 50, 630000) {
			st.SetCond(next)
		}
	case sporeZombie:
		if s.DropItems(p, carnSpore, 1, 50, 760000) {
			st.SetCond(next)
		}
	}
}
