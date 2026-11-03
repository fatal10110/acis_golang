// Package q299 is the quest Gather Ingredients for Pie: Emily sends a baker
// for a hundred honey pouches of the wasps, Lara's Avellan spice and
// Bright's fruit basket, and pays 25000 adena for the pie.
package q299

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q299_GatherIngredientsForPie"

// NPCs.
const (
	lara   = 30063
	bright = 30466
	emily  = 30620
)

// Items.
const (
	fruitBasket  = 7136
	avellanSpice = 7137
	honeyPouch   = 7138
)

const waspWorker = 20934

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Gather Ingredients for Pie",
		QuestID: 299,
		Items:   []int32{fruitBasket, avellanSpice, honeyPouch},
		Bind: script.Bindings{
			script.EventQuestStart: {emily},
			script.EventTalked:     {emily, lara, bright},
			script.EventMyDying:    {waspWorker, 20935}, // Wasp Worker, Wasp Leader
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
	case strings.EqualFold(e.Name, "30620-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30620-3.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, honeyPouch, -1)
	case strings.EqualFold(e.Name, "30063-1.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, avellanSpice, 1)
	case strings.EqualFold(e.Name, "30620-5.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, avellanSpice, 1)
	case strings.EqualFold(e.Name, "30466-1.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, fruitBasket, 1)
	case strings.EqualFold(e.Name, "30620-7a.htm"):
		if p.HasItem(fruitBasket) {
			htmltext = "30620-7.htm"
			s.TakeItems(p, fruitBasket, 1)
			s.RewardItems(p, 57, 25000)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			st.SetCond(5)
		}
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
		if p.Level() < 34 {
			htmltext = "30620-0a.htm"
		} else {
			htmltext = "30620-0.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case emily:
			switch cond {
			case 1:
				htmltext = "30620-1a.htm"
			case 2:
				if p.ItemCount(honeyPouch) >= 100 {
					htmltext = "30620-2.htm"
				} else {
					htmltext = "30620-2a.htm"
					st.Exit(true)
				}
			case 3:
				htmltext = "30620-3a.htm"
			case 4:
				if p.HasItem(avellanSpice) {
					htmltext = "30620-4.htm"
				} else {
					htmltext = "30620-4a.htm"
					st.Exit(true)
				}
			case 5:
				htmltext = "30620-5a.htm"
			case 6:
				htmltext = "30620-6.htm"
			}
		case lara:
			if cond == 3 {
				htmltext = "30063-0.htm"
			} else if cond > 3 {
				htmltext = "30063-1a.htm"
			}
		case bright:
			if cond == 5 {
				htmltext = "30466-0.htm"
			} else if cond > 5 {
				htmltext = "30466-1a.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	var chance int32 = 625000
	if e.NPC.NpcID() == waspWorker {
		chance = 571000
	}
	if s.DropItems(st.Player(), honeyPouch, 1, 100, chance) {
		st.SetCond(2)
	}
}
