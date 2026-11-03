// Package q621 is the quest Egg Delivery: Jeremy, for a character of level
// 68 or more, hands out five boiled eggs for the villagers Pulin, Naff,
// Crocus, Kuber and Beolin, each of whom pays a fee for one. The five fees
// go back to Jeremy and Valentine pays adena and a haste potion, or now and
// then a recipe.
package q621

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q621_EggDelivery"

// Items.
const (
	boiledEggs     = 7195
	feeOfBoiledEgg = 7196
)

// NPCs.
const (
	jeremy    = 31521
	pulin     = 31543
	naff      = 31544
	crocus    = 31545
	kuber     = 31546
	beolin    = 31547
	valentine = 31584
)

// Rewards.
const (
	adena       = 57
	hastePotion = 734
)

// rewards returns the recipes Valentine may hand out.
func rewards() []int32 { return []int32{6847, 6849, 6851} }

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Egg Delivery",
		QuestID: 621,
		Items:   []int32{boiledEggs, feeOfBoiledEgg},
		Bind: script.Bindings{
			script.EventQuestStart: {jeremy},
			script.EventTalked:     {jeremy, pulin, naff, crocus, kuber, beolin, valentine},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// deliver moves the quest to cond and swaps an egg of the player for the
// fee of the villager that was served.
func deliver(s *script.Script, e script.Event, st *script.QuestState, cond int32) {
	st.SetCond(cond)
	s.PlaySound(e.Player, script.SoundItemGet)
	s.TakeItems(e.Player, boiledEggs, 1)
	s.GiveItems(e.Player, feeOfBoiledEgg, 1)
}

// onAdvEvent starts the quest with five eggs, serves a villager, takes the
// five fees back to Jeremy (with fewer the quest ends), or collects
// Valentine's reward.
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
		s.GiveItems(e.Player, boiledEggs, 5)
	case strings.EqualFold(e.Name, "31543-02.htm"):
		deliver(s, e, st, 2)
	case strings.EqualFold(e.Name, "31544-02.htm"):
		deliver(s, e, st, 3)
	case strings.EqualFold(e.Name, "31545-02.htm"):
		deliver(s, e, st, 4)
	case strings.EqualFold(e.Name, "31546-02.htm"):
		deliver(s, e, st, 5)
	case strings.EqualFold(e.Name, "31547-02.htm"):
		deliver(s, e, st, 6)
	case strings.EqualFold(e.Name, "31521-06.htm"):
		if e.Player.ItemCount(feeOfBoiledEgg) < 5 {
			htmltext = "31521-08.htm"
			s.PlaySound(e.Player, script.SoundGiveUp)
			st.Exit(true)
		} else {
			st.SetCond(7)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, feeOfBoiledEgg, 5)
		}
	case strings.EqualFold(e.Name, "31584-02.htm"):
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
		eggs := e.Player.ItemCount(boiledEggs)
		switch e.NPC.NpcID() {
		case jeremy:
			switch cond {
			case 1:
				htmltext = "31521-04.htm"
			case 6:
				htmltext = "31521-05.htm"
			case 7:
				htmltext = "31521-07.htm"
			}
		case pulin:
			if cond == 1 && eggs == 5 {
				htmltext = "31543-01.htm"
			} else if cond > 1 {
				htmltext = "31543-03.htm"
			}
		case naff:
			if cond == 2 && eggs == 4 {
				htmltext = "31544-01.htm"
			} else if cond > 2 {
				htmltext = "31544-03.htm"
			}
		case crocus:
			if cond == 3 && eggs == 3 {
				htmltext = "31545-01.htm"
			} else if cond > 3 {
				htmltext = "31545-03.htm"
			}
		case kuber:
			if cond == 4 && eggs == 2 {
				htmltext = "31546-01.htm"
			} else if cond > 4 {
				htmltext = "31546-03.htm"
			}
		case beolin:
			if cond == 5 && eggs == 1 {
				htmltext = "31547-01.htm"
			} else if cond > 5 {
				htmltext = "31547-03.htm"
			}
		case valentine:
			if cond == 7 {
				htmltext = "31584-01.htm"
			}
		}
	}
	return htmltext
}
