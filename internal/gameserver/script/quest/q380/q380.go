// Package q380 is the quest Bring Out the Flavor of Ingredients: Rollant
// asks for fruit, flowers and leech fluids, then for a jelly made of them.
package q380

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q380_BringOutTheFlavorOfIngredients"

// Npcs.
const rollant = 30069

// Monsters.
const (
	direWolf       = 20205
	kadifWerewolf  = 20206
	giantMistLeech = 20225
)

// Quest items.
const (
	ritronFruit    = 5895
	moonFaceFlower = 5896
	leechFluids    = 5897
	antidote       = 1831
)

// Rewards.
const (
	ritronJelly = 5960
	jellyRecipe = 5959
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Bring Out the Flavor of Ingredients!",
		QuestID: 380,
		Items:   []int32{ritronFruit, moonFaceFlower, leechFluids},
		Bind: script.Bindings{
			script.EventQuestStart: {rollant},
			script.EventTalked:     {rollant},
			script.EventMyDying:    {direWolf, kadifWerewolf, giantMistLeech},
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
	case strings.EqualFold(e.Name, "30069-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30069-12.htm"):
		s.GiveItems(p, jellyRecipe, 1)
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
		if p.Level() < 24 {
			htmltext = "30069-00.htm"
		} else {
			htmltext = "30069-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30069-06.htm"
		case 2:
			if p.ItemCount(antidote) >= 2 {
				htmltext = "30069-07.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, ritronFruit, -1)
				s.TakeItems(p, moonFaceFlower, -1)
				s.TakeItems(p, leechFluids, -1)
				s.TakeItems(p, antidote, 2)
			} else {
				htmltext = "30069-06.htm"
			}
		case 3:
			htmltext = "30069-08.htm"
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
		case 4:
			htmltext = "30069-09.htm"
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
		case 5:
			htmltext = "30069-10.htm"
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
		case 6:
			s.GiveItems(p, ritronJelly, 1)
			if s.Rnd(100) < 55 {
				htmltext = "30069-11.htm"
			} else {
				htmltext = "30069-13.htm"
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case direWolf:
		if s.DropItems(p, ritronFruit, 1, 4, 100000) {
			if p.ItemCount(moonFaceFlower) == 20 && p.ItemCount(leechFluids) == 10 {
				st.SetCond(2)
			}
		}
	case kadifWerewolf:
		if s.DropItems(p, moonFaceFlower, 1, 20, 500000) {
			if p.ItemCount(ritronFruit) == 4 && p.ItemCount(leechFluids) == 10 {
				st.SetCond(2)
			}
		}
	case giantMistLeech:
		if s.DropItems(p, leechFluids, 1, 10, 500000) {
			if p.ItemCount(ritronFruit) == 4 && p.ItemCount(moonFaceFlower) == 20 {
				st.SetCond(2)
			}
		}
	}
}
