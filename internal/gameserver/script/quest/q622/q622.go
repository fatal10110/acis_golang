// Package q622 is the quest Specialty Liquor Delivery: Jeremy, for a
// character of level 68 or more, hands out five bottles of special drink
// for the villagers Beolin, Kuber, Crocus, Naff and Pulin, each of whom pays
// a fee for one. The five fees go back to Jeremy and Lietta pays adena and a
// haste potion, or now and then a recipe.
package q622

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q622_SpecialtyLiquorDelivery"

// Items.
const (
	specialDrink      = 7197
	feeOfSpecialDrink = 7198
)

// NPCs.
const (
	jeremy = 31521
	pulin  = 31543
	naff   = 31544
	crocus = 31545
	kuber  = 31546
	beolin = 31547
	lietta = 31267
)

// Rewards.
const (
	adena       = 57
	hastePotion = 734
)

// rewards returns the recipes Lietta may hand out.
func rewards() []int32 { return []int32{6847, 6849, 6851} }

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Specialty Liquor Delivery",
		QuestID: 622,
		Items:   []int32{specialDrink, feeOfSpecialDrink},
		Bind: script.Bindings{
			script.EventQuestStart: {jeremy},
			script.EventTalked:     {jeremy, pulin, naff, crocus, kuber, beolin, lietta},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// deliver moves the quest to cond and swaps a bottle of the player for the
// fee of the villager that was served.
func deliver(s *script.Script, e script.Event, st *script.QuestState, cond int32) {
	st.SetCond(cond)
	s.PlaySound(e.Player, script.SoundMiddle)
	s.TakeItems(e.Player, specialDrink, 1)
	s.GiveItems(e.Player, feeOfSpecialDrink, 1)
}

// onAdvEvent starts the quest with five bottles, serves a villager, takes
// the five fees back to Jeremy, or collects Lietta's reward.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31521-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, specialDrink, 5)
	case strings.EqualFold(e.Name, "31547-02.htm"):
		deliver(s, e, st, 2)
	case strings.EqualFold(e.Name, "31546-02.htm"):
		deliver(s, e, st, 3)
	case strings.EqualFold(e.Name, "31545-02.htm"):
		deliver(s, e, st, 4)
	case strings.EqualFold(e.Name, "31544-02.htm"):
		deliver(s, e, st, 5)
	case strings.EqualFold(e.Name, "31543-02.htm"):
		deliver(s, e, st, 6)
	case strings.EqualFold(e.Name, "31521-06.htm"):
		st.SetCond(7)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, feeOfSpecialDrink, 5)
	case strings.EqualFold(e.Name, "31267-02.htm"):
		if s.Rnd(5) < 1 {
			rs := rewards()
			s.GiveItems(e.Player, rs[s.Rnd(len(rs))], 1)
		} else {
			s.RewardItems(e.Player, adena, 18800)
			s.RewardItems(e.Player, hastePotion, 1)
		}
		s.PlaySound(e.Player, script.SoundFinish)
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 68 {
			htmltext = "31521-03.htm"
		} else {
			htmltext = "31521-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		drinks := e.Player.ItemCount(specialDrink)
		switch e.NPC.NpcID() {
		case jeremy:
			switch {
			case cond < 6:
				htmltext = "31521-04.htm"
			case cond == 6:
				htmltext = "31521-05.htm"
			case cond == 7:
				htmltext = "31521-06.htm"
			}
		case beolin:
			if cond == 1 && drinks == 5 {
				htmltext = "31547-01.htm"
			} else if cond > 1 {
				htmltext = "31547-03.htm"
			}
		case kuber:
			if cond == 2 && drinks == 4 {
				htmltext = "31546-01.htm"
			} else if cond > 2 {
				htmltext = "31546-03.htm"
			}
		case crocus:
			if cond == 3 && drinks == 3 {
				htmltext = "31545-01.htm"
			} else if cond > 3 {
				htmltext = "31545-03.htm"
			}
		case naff:
			if cond == 4 && drinks == 2 {
				htmltext = "31544-01.htm"
			} else if cond > 4 {
				htmltext = "31544-03.htm"
			}
		case pulin:
			if cond == 5 && drinks == 1 {
				htmltext = "31543-01.htm"
			} else if cond > 5 {
				htmltext = "31543-03.htm"
			}
		case lietta:
			if cond == 7 {
				htmltext = "31267-01.htm"
			}
		}
	}
	return htmltext
}
