// Package q030 is the quest Chest caught with a bait of fire: once
// Linnaeus' Special Bait is done, Linnaeus opens a red treasure box the
// player fished up and finds a musical score, which Rukal trades for a
// necklace of protection.
package q030

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q030_ChestCaughtWithABaitOfFire"

// NPCs.
const (
	linnaeus = 31577
	rukal    = 30629
)

// Items.
const (
	redTreasureBox       = 6511
	musicalScore         = 7628
	necklaceOfProtection = 916
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Chest caught with a bait of fire",
		QuestID: 30,
		Items:   []int32{musicalScore},
		Bind: script.Bindings{
			script.EventQuestStart: {linnaeus},
			script.EventTalked:     {linnaeus, rukal},
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
	case strings.EqualFold(e.Name, "31577-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31577-07.htm"):
		if p.HasItem(redTreasureBox) {
			st.SetCond(2)
			s.TakeItems(p, redTreasureBox, 1)
			s.GiveItems(p, musicalScore, 1)
		} else {
			htmltext = "31577-08.htm"
		}
	case strings.EqualFold(e.Name, "30629-02.htm"):
		if p.HasItem(musicalScore) {
			htmltext = "30629-02.htm"
			s.TakeItems(p, musicalScore, 1)
			s.GiveItems(p, necklaceOfProtection, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30629-03.htm"
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
		if p.Level() < 60 {
			htmltext = "31577-02.htm"
		} else if st2 := s.QuestState(p, "Q053_LinnaeusSpecialBait"); st2 != nil && st2.IsCompleted() {
			htmltext = "31577-01.htm"
		} else {
			htmltext = "31577-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case linnaeus:
			if cond == 1 {
				if !p.HasItem(redTreasureBox) {
					htmltext = "31577-06.htm"
				} else {
					htmltext = "31577-05.htm"
				}
			} else if cond == 2 {
				htmltext = "31577-09.htm"
			}
		case rukal:
			if cond == 2 {
				htmltext = "30629-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
