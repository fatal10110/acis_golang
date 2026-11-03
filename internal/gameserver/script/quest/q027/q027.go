// Package q027 is the quest Chest caught with a bait of wind: once Lanosco's
// Special Bait is done, Lanosco opens a large blue treasure chest the player
// fished up and finds a strange blueprint, which Shaling trades for a black
// pearl ring.
package q027

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q027_ChestCaughtWithABaitOfWind"

// NPCs.
const (
	lanosco = 31570
	shaling = 31434
)

// Items.
const (
	largeBlueTreasureChest = 6500
	strangeBlueprint       = 7625
	blackPearlRing         = 880
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Chest caught with a bait of wind",
		QuestID: 27,
		Items:   []int32{strangeBlueprint},
		Bind: script.Bindings{
			script.EventQuestStart: {lanosco},
			script.EventTalked:     {lanosco, shaling},
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
	case strings.EqualFold(e.Name, "31570-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31570-07.htm"):
		if p.HasItem(largeBlueTreasureChest) {
			st.SetCond(2)
			s.TakeItems(p, largeBlueTreasureChest, 1)
			s.GiveItems(p, strangeBlueprint, 1)
		} else {
			htmltext = "31570-08.htm"
		}
	case strings.EqualFold(e.Name, "31434-02.htm"):
		if p.HasItem(strangeBlueprint) {
			htmltext = "31434-02.htm"
			s.TakeItems(p, strangeBlueprint, 1)
			s.GiveItems(p, blackPearlRing, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "31434-03.htm"
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
		if p.Level() < 27 {
			htmltext = "31570-02.htm"
		} else if st2 := s.QuestState(p, "Q050_LanoscosSpecialBait"); st2 != nil && st2.IsCompleted() {
			htmltext = "31570-01.htm"
		} else {
			htmltext = "31570-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case lanosco:
			if cond == 1 {
				if !p.HasItem(largeBlueTreasureChest) {
					htmltext = "31570-06.htm"
				} else {
					htmltext = "31570-05.htm"
				}
			} else if cond == 2 {
				htmltext = "31570-09.htm"
			}
		case shaling:
			if cond == 2 {
				htmltext = "31434-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
