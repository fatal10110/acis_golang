// Package q043 is the quest Help the Sister!: Cooper trades a crafted dagger
// for the task of gathering thirty map pieces from specters and sorrow maidens;
// Galladucci takes the map, and Cooper pays with a pet ticket.
package q043

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q043_HelpTheSister"

// NPCs.
const (
	cooper     = 30829
	galladucci = 30097
)

// Items.
const (
	craftedDagger = 220
	mapPiece      = 7550
	questMap      = 7551
	petTicket     = 7584
)

// Monsters.
const (
	specter      = 20171
	sorrowMaiden = 20197
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Help the Sister!",
		QuestID: 43,
		Items:   []int32{mapPiece, questMap},
		Bind: script.Bindings{
			script.EventQuestStart: {cooper},
			script.EventTalked:     {cooper, galladucci},
			script.EventMyDying:    {specter, sorrowMaiden},
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
	case strings.EqualFold(e.Name, "30829-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30829-03.htm") && p.HasItem(craftedDagger):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, craftedDagger, 1)
	case strings.EqualFold(e.Name, "30829-05.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, mapPiece, 30)
		s.GiveItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "30097-06.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, questMap, 1)
	case strings.EqualFold(e.Name, "30829-07.htm"):
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
		if p.Level() < 26 {
			htmltext = "30829-00a.htm"
		} else {
			htmltext = "30829-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case cooper:
			switch cond {
			case 1:
				if !p.HasItem(craftedDagger) {
					htmltext = "30829-01a.htm"
				} else {
					htmltext = "30829-02.htm"
				}
			case 2:
				htmltext = "30829-03a.htm"
			case 3:
				htmltext = "30829-04.htm"
			case 4:
				htmltext = "30829-05a.htm"
			case 5:
				htmltext = "30829-06.htm"
			}
		case galladucci:
			if cond == 4 {
				htmltext = "30097-05.htm"
			} else if cond == 5 {
				htmltext = "30097-06a.htm"
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
