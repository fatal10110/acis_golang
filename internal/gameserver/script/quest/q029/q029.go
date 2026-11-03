// Package q029 is the quest Chest caught with a bait of earth: once Willie's
// Special Bait is done, Willie opens a small purple treasure chest the player
// fished up and finds a small glass box, which Anabel trades for plated
// leather gloves.
package q029

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q029_ChestCaughtWithABaitOfEarth"

// NPCs.
const (
	willie = 31574
	anabel = 30909
)

// Items.
const (
	smallPurpleTreasureChest = 6507
	smallGlassBox            = 7627
	platedLeatherGloves      = 2455
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Chest caught with a bait of earth",
		QuestID: 29,
		Items:   []int32{smallGlassBox},
		Bind: script.Bindings{
			script.EventQuestStart: {willie},
			script.EventTalked:     {willie, anabel},
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
	case strings.EqualFold(e.Name, "31574-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31574-07.htm"):
		if p.HasItem(smallPurpleTreasureChest) {
			st.SetCond(2)
			s.TakeItems(p, smallPurpleTreasureChest, 1)
			s.GiveItems(p, smallGlassBox, 1)
		} else {
			htmltext = "31574-08.htm"
		}
	case strings.EqualFold(e.Name, "30909-02.htm"):
		if p.HasItem(smallGlassBox) {
			htmltext = "30909-02.htm"
			s.TakeItems(p, smallGlassBox, 1)
			s.GiveItems(p, platedLeatherGloves, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30909-03.htm"
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
		if p.Level() < 48 {
			htmltext = "31574-02.htm"
		} else if st2 := s.QuestState(p, "Q052_WilliesSpecialBait"); st2 != nil && st2.IsCompleted() {
			htmltext = "31574-01.htm"
		} else {
			htmltext = "31574-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case willie:
			if cond == 1 {
				if !p.HasItem(smallPurpleTreasureChest) {
					htmltext = "31574-06.htm"
				} else {
					htmltext = "31574-05.htm"
				}
			} else if cond == 2 {
				htmltext = "31574-09.htm"
			}
		case anabel:
			if cond == 2 {
				htmltext = "30909-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
