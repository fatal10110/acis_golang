// Package q002 is the quest What Women Want: Arujien's letter goes to
// Mirabel, then Herbiel; Arujien then either pays adena for it or sends a
// poetry book to Greenis, whose letter back earns a mystic's earring.
package q002

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q002_WhatWomenWant"

// NPCs.
const (
	arujien = 30223
	mirabel = 30146
	herbiel = 30150
	greenis = 30157
)

// Items.
const (
	arujienLetter1 = 1092
	arujienLetter2 = 1093
	arujienLetter3 = 1094
	poetryBook     = 689
	greenisLetter  = 693
)

// Reward.
const mysticsEarring = 113

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "What Women Want",
		QuestID: 2,
		Items:   []int32{arujienLetter1, arujienLetter2, arujienLetter3, poetryBook, greenisLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {arujien},
			script.EventTalked:     {arujien, mirabel, herbiel, greenis},
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
	switch {
	case strings.EqualFold(e.Name, "30223-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, arujienLetter1, 1)
	case strings.EqualFold(e.Name, "30223-08.htm"):
		st.SetCond(4)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, arujienLetter3, 1)
		s.GiveItems(e.Player, poetryBook, 1)
	case strings.EqualFold(e.Name, "30223-09.htm"):
		s.TakeItems(e.Player, arujienLetter3, 1)
		s.RewardItems(e.Player, 57, 450)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(false)
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
		switch {
		case e.Player.Race() != player.RaceElf && e.Player.Race() != player.RaceHuman:
			htmltext = "30223-00.htm"
		case e.Player.Level() < 2:
			htmltext = "30223-01.htm"
		default:
			htmltext = "30223-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case arujien:
			switch {
			case e.Player.HasItem(arujienLetter1):
				htmltext = "30223-05.htm"
			case e.Player.HasItem(arujienLetter3):
				htmltext = "30223-07.htm"
			case e.Player.HasItem(arujienLetter2):
				htmltext = "30223-06.htm"
			case e.Player.HasItem(poetryBook):
				htmltext = "30223-11.htm"
			case e.Player.HasItem(greenisLetter):
				htmltext = "30223-10.htm"
				s.TakeItems(e.Player, greenisLetter, 1)
				s.GiveItems(e.Player, mysticsEarring, 1)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			}
		case mirabel:
			switch {
			case cond == 1:
				htmltext = "30146-01.htm"
				st.SetCond(2)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, arujienLetter1, 1)
				s.GiveItems(e.Player, arujienLetter2, 1)
			case cond > 1:
				htmltext = "30146-02.htm"
			}
		case herbiel:
			switch {
			case cond == 2:
				htmltext = "30150-01.htm"
				st.SetCond(3)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, arujienLetter2, 1)
				s.GiveItems(e.Player, arujienLetter3, 1)
			case cond > 2:
				htmltext = "30150-02.htm"
			}
		case greenis:
			switch {
			case cond < 4:
				htmltext = "30157-01.htm"
			case cond == 4:
				htmltext = "30157-02.htm"
				st.SetCond(5)
				s.PlaySound(e.Player, script.SoundMiddle)
				s.TakeItems(e.Player, poetryBook, 1)
				s.GiveItems(e.Player, greenisLetter, 1)
			case cond == 5:
				htmltext = "30157-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
