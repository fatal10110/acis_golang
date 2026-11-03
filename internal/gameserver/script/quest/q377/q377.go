// Package q377 is the quest Exploration of the Giants' Cave, Part 2: Sobling
// turns the books of the Titans into recipes.
package q377

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q377_ExplorationOfTheGiantsCave_Part2"

// Npcs.
const sobling = 31147

// Quest items.
const (
	ancientTitanBook                   = 5955
	ancientDictionaryIntermediateLevel = 5892
)

// books lists the five books of each subject.
func books() [][]int32 {
	return [][]int32{
		{5945, 5946, 5947, 5948, 5949},
		{5950, 5951, 5952, 5953, 5954},
	}
}

// recipes lists the recipes a complete subject may be traded for, in the
// order of books.
func recipes() [][]int32 {
	return [][]int32{
		{5338, 5336},
		{5420, 5422},
	}
}

// chance returns the drop chance of the Titan book for a monster.
func chance(npcID int32) int32 {
	switch npcID {
	case 20654:
		return 25000
	case 20656:
		return 22000
	case 20657:
		return 16000
	case 20658:
		return 15000
	}
	panic(npcID)
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Exploration of the Giants' Cave, Part 2",
		QuestID: 377,
		Bind: script.Bindings{
			script.EventQuestStart: {sobling},
			script.EventTalked:     {sobling},
			script.EventMyDying:    {20654, 20656, 20657, 20658},
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
	case strings.EqualFold(e.Name, "31147-04.htm"):
		htmltext = checkItems(s, p)
	case strings.EqualFold(e.Name, "31147-07.htm"):
		s.PlaySound(p, script.SoundFinish)
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 57 || !p.HasItem(ancientDictionaryIntermediateLevel) {
			htmltext = "31147-01.htm"
		} else {
			htmltext = "31147-02.htm"
		}
	case questlog.StatusStarted:
		htmltext = checkItems(s, p)
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), ancientTitanBook, 1, 0, chance(e.NPC.NpcID()))
}

func checkItems(s *script.Script, p *script.Player) string {
	for subject, set := range books() {
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
