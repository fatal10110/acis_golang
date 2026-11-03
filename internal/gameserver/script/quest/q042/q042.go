// Package q042 is the quest Help the Uncle!: Waters trades a trident for
// the task of gathering thirty map pieces from the monster eyes; Sophya takes
// the map, and Waters pays with a pet ticket.
package q042

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q042_HelpTheUncle"

// NPCs.
const (
	waters = 30828
	sophya = 30735
)

// Items.
const (
	trident   = 291
	mapPiece  = 7548
	questMap  = 7549
	petTicket = 7583
)

// Monsters.
const (
	monsterEyeDestroyer = 20068
	monsterEyeGazer     = 20266
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Help the Uncle!",
		QuestID: 42,
		Items:   []int32{mapPiece, questMap},
		Bind: script.Bindings{
			script.EventQuestStart: {waters},
			script.EventTalked:     {waters, sophya},
			script.EventMyDying:    {monsterEyeDestroyer, monsterEyeGazer},
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
	case strings.EqualFold(e.Name, "30828-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30828-03.htm") && p.HasItem(trident):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, trident, 1)
	case strings.EqualFold(e.Name, "30828-05.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mapPiece, 30)
		s.GiveItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "30735-06.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "30828-07.htm"):
		s.GiveItems(p, petTicket, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		if p.Level() < 25 {
			htmltext = "30828-00a.htm"
		} else {
			htmltext = "30828-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case waters:
			switch cond {
			case 1:
				if !p.HasItem(trident) {
					htmltext = "30828-01a.htm"
				} else {
					htmltext = "30828-02.htm"
				}
			case 2:
				htmltext = "30828-03a.htm"
			case 3:
				htmltext = "30828-04.htm"
			case 4:
				htmltext = "30828-05a.htm"
			case 5:
				htmltext = "30828-06.htm"
			}
		case sophya:
			if cond == 4 {
				htmltext = "30735-05.htm"
			} else if cond == 5 {
				htmltext = "30735-06a.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		return
	}
	if s.DropItemsAlways(p, mapPiece, 1, 30) {
		st.SetCond(3)
	}
}
