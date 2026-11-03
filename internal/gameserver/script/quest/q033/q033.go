// Package q033 is the quest Make a Pair of Dress Shoes: for a player whose
// formal wear is waiting on shoes, Woodley has Leikar's measure taken, then
// leather, thread and adena, and Ian's work, for a dress shoes box.
package q033

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q033_MakeAPairOfDressShoes"

// NPCs.
const (
	woodley = 30838
	ian     = 30164
	leikar  = 31520
)

// Items.
const (
	leather = 1882
	thread  = 1868
	adena   = 57
)

// Reward.
const dressShoesBox = 7113

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Make a Pair of Dress Shoes",
		QuestID: 33,
		Bind: script.Bindings{
			script.EventQuestStart: {woodley},
			script.EventTalked:     {woodley, ian, leikar},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
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
	case strings.EqualFold(e.Name, "30838-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31520-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30838-3.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30838-5.htm"):
		if hasMaterials(p) {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, adena, 200000)
			s.TakeItems(p, leather, 200)
			s.TakeItems(p, thread, 600)
		} else {
			htmltext = "30838-4a.htm"
		}
	case strings.EqualFold(e.Name, "30164-1.htm"):
		if p.ItemCount(adena) >= 300000 {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, adena, 300000)
		} else {
			htmltext = "30164-1a.htm"
		}
	case strings.EqualFold(e.Name, "30838-7.htm"):
		s.GiveItems(p, dressShoesBox, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

// hasMaterials reports whether the player holds the leather, thread and
// adena Woodley asks for.
func hasMaterials(p *script.Player) bool {
	return p.ItemCount(leather) >= 200 && p.ItemCount(thread) >= 600 && p.ItemCount(adena) >= 200000
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
		if p.Level() >= 60 {
			if fwear := s.QuestState(p, "Q037_MakeFormalWear"); fwear != nil && fwear.Cond() == 7 {
				htmltext = "30838-0.htm"
			} else {
				htmltext = "30838-0a.htm"
			}
		} else {
			htmltext = "30838-0b.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case woodley:
			switch cond {
			case 1:
				htmltext = "30838-1.htm"
			case 2:
				htmltext = "30838-2.htm"
			case 3:
				if hasMaterials(p) {
					htmltext = "30838-4.htm"
				} else {
					htmltext = "30838-4a.htm"
				}
			case 4:
				htmltext = "30838-5a.htm"
			case 5:
				htmltext = "30838-6.htm"
			}
		case leikar:
			if cond == 1 {
				htmltext = "31520-0.htm"
			} else if cond > 1 {
				htmltext = "31520-1a.htm"
			}
		case ian:
			if cond == 4 {
				htmltext = "30164-0.htm"
			} else if cond == 5 {
				htmltext = "30164-2.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
