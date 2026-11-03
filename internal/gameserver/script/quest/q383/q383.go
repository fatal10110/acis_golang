// Package q383 is the quest Searching for Treasure: Espen deciphers a pirate
// treasure map that leads to a chest opened with a thief's key.
package q383

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q383_SearchingForTreasure"

// Npcs.
const (
	espen       = 30890
	pirateChest = 31148
)

// Items.
const (
	pirateTreasureMap = 5915
	thiefKey          = 1661
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Searching for Treasure",
		QuestID: 383,
		Bind: script.Bindings{
			script.EventQuestStart: {espen},
			script.EventTalked:     {espen, pirateChest},
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
	case strings.EqualFold(e.Name, "30890-04.htm"):
		// Sell the map.
		if p.HasItem(pirateTreasureMap) {
			s.TakeItems(p, pirateTreasureMap, 1)
			s.RewardItems(p, 57, 1000)
		} else {
			htmltext = "30890-06.htm"
		}
	case strings.EqualFold(e.Name, "30890-07.htm"):
		// Listen the story.
		if p.HasItem(pirateTreasureMap) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
		} else {
			htmltext = "30890-06.htm"
		}
	case strings.EqualFold(e.Name, "30890-11.htm"):
		// Decipher the map.
		if p.HasItem(pirateTreasureMap) {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, pirateTreasureMap, 1)
		} else {
			htmltext = "30890-06.htm"
		}
	case strings.EqualFold(e.Name, "31148-02.htm"):
		if p.HasItem(thiefKey) {
			s.TakeItems(p, thiefKey, 1)
			openChest(s, p)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31148-03.htm"
		}
	}
	return htmltext
}

// openChest gives the four independent rolls of the chest and the adena of
// the rolls that gave no item.
func openChest(s *script.Script, p *script.Player) {
	// Adena reward.
	var i1 int32

	i0 := s.Rnd(100)
	if i0 < 5 {
		s.GiveItems(p, 2450, 1)
	} else if i0 < 6 {
		s.GiveItems(p, 2451, 1)
	} else if i0 < 18 {
		s.GiveItems(p, 956, 1)
	} else if i0 < 28 {
		s.GiveItems(p, 952, 1)
	} else {
		i1 += 500
	}

	i0 = s.Rnd(1000)
	if i0 < 25 {
		s.GiveItems(p, 4481, 1)
	} else if i0 < 50 {
		s.GiveItems(p, 4482, 1)
	} else if i0 < 75 {
		s.GiveItems(p, 4483, 1)
	} else if i0 < 100 {
		s.GiveItems(p, 4484, 1)
	} else if i0 < 125 {
		s.GiveItems(p, 4485, 1)
	} else if i0 < 150 {
		s.GiveItems(p, 4486, 1)
	} else if i0 < 175 {
		s.GiveItems(p, 4487, 1)
	} else if i0 < 200 {
		s.GiveItems(p, 4488, 1)
	} else if i0 < 225 {
		s.GiveItems(p, 4489, 1)
	} else if i0 < 250 {
		s.GiveItems(p, 4490, 1)
	} else if i0 < 275 {
		s.GiveItems(p, 4491, 1)
	} else if i0 < 300 {
		s.GiveItems(p, 4492, 1)
	} else {
		i1 += 300
	}

	i0 = s.Rnd(100)
	if i0 < 4 {
		s.GiveItems(p, 1337, 1)
	} else if i0 < 8 {
		s.GiveItems(p, 1338, 2)
	} else if i0 < 12 {
		s.GiveItems(p, 1339, 2)
	} else if i0 < 16 {
		s.GiveItems(p, 3447, 2)
	} else if i0 < 20 {
		s.GiveItems(p, 3450, 1)
	} else if i0 < 25 {
		s.GiveItems(p, 3453, 1)
	} else if i0 < 27 {
		s.GiveItems(p, 3456, 1)
	} else {
		i1 += 500
	}

	i0 = s.Rnd(100)
	if i0 < 20 {
		s.GiveItems(p, 4408, 1)
	} else if i0 < 40 {
		s.GiveItems(p, 4409, 1)
	} else if i0 < 60 {
		s.GiveItems(p, 4418, 1)
	} else if i0 < 80 {
		s.GiveItems(p, 4419, 1)
	} else {
		i1 += 500
	}

	s.RewardItems(p, 57, i1)
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 42 || !e.Player.HasItem(pirateTreasureMap) {
			htmltext = "30890-01.htm"
		} else {
			htmltext = "30890-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case espen:
			if cond == 1 {
				htmltext = "30890-07a.htm"
			} else {
				htmltext = "30890-12.htm"
			}
		case pirateChest:
			if cond == 2 {
				htmltext = "31148-01.htm"
			}
		}
	}
	return htmltext
}
