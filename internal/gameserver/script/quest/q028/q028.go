// Package q028 is the quest Chest caught with a bait of icy air: once
// O'Fulle's Special Bait is done, O'Fulle opens a big yellow treasure chest
// the player fished up and finds Kiki's letter, which Kiki trades for an
// elven ring.
package q028

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q028_ChestCaughtWithABaitOfIcyAir"

// NPCs.
const (
	ofulle = 31572
	kiki   = 31442
)

// Items.
const (
	bigYellowTreasureChest = 6503
	kikiLetter             = 7626
	elvenRing              = 881
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Chest caught with a bait of icy air",
		QuestID: 28,
		Items:   []int32{kikiLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {ofulle},
			script.EventTalked:     {ofulle, kiki},
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
	case strings.EqualFold(e.Name, "31572-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31572-07.htm"):
		if p.HasItem(bigYellowTreasureChest) {
			st.SetCond(2)
			s.TakeItems(p, bigYellowTreasureChest, 1)
			s.GiveItems(p, kikiLetter, 1)
		} else {
			htmltext = "31572-08.htm"
		}
	case strings.EqualFold(e.Name, "31442-02.htm"):
		if p.HasItem(kikiLetter) {
			htmltext = "31442-02.htm"
			s.TakeItems(p, kikiLetter, 1)
			s.GiveItems(p, elvenRing, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "31442-03.htm"
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
		if p.Level() < 36 {
			htmltext = "31572-02.htm"
		} else if st2 := s.QuestState(p, "Q051_OFullesSpecialBait"); st2 != nil && st2.IsCompleted() {
			htmltext = "31572-01.htm"
		} else {
			htmltext = "31572-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case ofulle:
			if cond == 1 {
				if !p.HasItem(bigYellowTreasureChest) {
					htmltext = "31572-06.htm"
				} else {
					htmltext = "31572-05.htm"
				}
			} else if cond == 2 {
				htmltext = "31572-09.htm"
			}
		case kiki:
			if cond == 2 {
				htmltext = "31442-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
