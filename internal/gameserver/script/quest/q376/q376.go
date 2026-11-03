// Package q376 is the quest Exploration of the Giants' Cave, Part 1: Sobling
// turns the ancient parchments of the cave into recipes, and Cliff reads the
// mysterious book.
package q376

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q376_ExplorationOfTheGiantsCave_Part1"

// Npcs.
const (
	sobling = 31147
	cliff   = 30182
)

// Quest items.
const (
	ancientParchment                   = 5944
	mysteriousBook                     = 5890
	ancientDictionaryBasicLevel        = 5891
	ancientDictionaryIntermediateLevel = 5892
)

// books lists the five books of each subject.
func books() [][]int32 {
	return [][]int32{
		{5937, 5938, 5939, 5940, 5941},
		{5932, 5933, 5934, 5935, 5936},
		{5922, 5923, 5924, 5925, 5926},
		{5927, 5928, 5929, 5930, 5931},
	}
}

// recipes lists the recipes a complete subject may be traded for, in the
// order of books.
func recipes() [][]int32 {
	return [][]int32{
		{5346, 5354},
		{5332, 5334},
		{5416, 5418},
		{5424, 5340},
	}
}

// chance returns the drop chance of the ancient parchment for a monster.
func chance(npcID int32) int32 {
	switch npcID {
	case 20647:
		return 26000
	case 20648:
		return 28000
	case 20649:
		return 30000
	case 20650:
		return 32000
	}
	panic(npcID)
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Exploration of the Giants' Cave, Part 1",
		QuestID: 376,
		Items:   []int32{ancientDictionaryBasicLevel, mysteriousBook},
		Bind: script.Bindings{
			script.EventQuestStart: {sobling},
			script.EventTalked:     {sobling, cliff},
			script.EventMyDying:    {20647, 20648, 20649, 20650},
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
	case strings.EqualFold(e.Name, "31147-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ancientDictionaryBasicLevel, 1)
	case strings.EqualFold(e.Name, "31147-04.htm"):
		htmltext = checkItems(s, p, st)
	case strings.EqualFold(e.Name, "31147-09.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30182-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mysteriousBook, -1)
		s.GiveItems(p, ancientDictionaryIntermediateLevel, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 51 {
			htmltext = "31147-01.htm"
		} else {
			htmltext = "31147-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case sobling:
			htmltext = checkItems(s, p, st)
		case cliff:
			if cond == 2 && p.HasItem(mysteriousBook) {
				htmltext = "30182-01.htm"
			} else if cond == 3 {
				htmltext = "30182-03.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	// The book goes to those who still need it.
	if !st.Player().HasAtLeastOneItem(mysteriousBook, ancientDictionaryIntermediateLevel) {
		s.DropItems(st.Player(), mysteriousBook, 1, 1, 2000)
	}
	// The parchment goes to anyone.
	s.DropItems(st.Player(), ancientParchment, 1, 0, chance(e.NPC.NpcID()))
}

func checkItems(s *script.Script, p *script.Player, st *script.QuestState) string {
	if p.HasItem(mysteriousBook) {
		if st.Cond() == 1 {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			return "31147-07.htm"
		}
		return "31147-08.htm"
	}
	shelf := books()
	for subject, set := range shelf {
		complete := true
		for _, book := range set {
			if !p.HasItem(book) {
				complete = false
			}
		}
		if complete {
			for _, book := range set {
				s.TakeItems(p, book, 1)
			}
			choices := recipes()[subject]
			s.GiveItems(p, choices[s.Rnd(len(choices))], 1)
			return "31147-04.htm"
		}
	}
	return "31147-05.htm"
}
