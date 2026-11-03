// Package q359 is the quest For Sleepless Deadmen: Orven wants the remains of
// the doom servants, guards and archers.
package q359

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q359_ForSleeplessDeadmen"

// orven is the quest giver.
const orven = 30857

// remains is the quest item.
const remains = 5869

// Monsters.
const (
	doomServant = 21006
	doomGuard   = 21007
	doomArcher  = 21008
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "For Sleepless Deadmen",
		QuestID: 359,
		Items:   []int32{remains},
		Bind: script.Bindings{
			script.EventQuestStart: {orven},
			script.EventTalked:     {orven},
			script.EventMyDying:    {doomServant, doomGuard, doomArcher},
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
	switch {
	case strings.EqualFold(e.Name, "30857-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30857-10.htm"):
		rewards := []int32{6341, 6342, 6343, 6344, 6345, 6346, 5494, 5495}
		s.GiveItems(e.Player, rewards[s.Rnd(len(rewards))], 4)
		s.PlaySound(e.Player, script.SoundFinish)
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
		if p.Level() < 60 {
			htmltext = "30857-01.htm"
		} else {
			htmltext = "30857-02.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30857-07.htm"
		case 2:
			htmltext = "30857-08.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, remains, -1)
		case 3:
			htmltext = "30857-09.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, remains, 1, 60, chance(e.NPC.NpcID())) {
		st.SetCond(2)
	}
}

// chance is the per-monster drop chance of the remains.
func chance(npcID int32) int32 {
	switch npcID {
	case doomServant:
		return 320000
	case doomGuard:
		return 340000
	default:
		return 420000
	}
}
