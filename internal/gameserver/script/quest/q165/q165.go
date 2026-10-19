// Package q165 is the quest Shilen's Hunt: Nelsya rewards a dark elf for
// thirteen dark bezoars of the wolves and keltirs.
package q165

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q165_ShilensHunt"

// Monsters.
const (
	ashenWolf        = 20456
	youngBrownKeltir = 20529
	brownKeltir      = 20532
	elderBrownKeltir = 20536
)

// Items.
const (
	darkBezoar          = 1160
	lesserHealingPotion = 1060
)

// chances returns the dark bezoar drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		ashenWolf:        1000000,
		youngBrownKeltir: 333333,
		brownKeltir:      333333,
		elderBrownKeltir: 666667,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Shilen's Hunt",
		QuestID: 165,
		Items:   []int32{darkBezoar},
		Bind: script.Bindings{
			script.EventQuestStart: {30348}, // Nelsya
			script.EventTalked:     {30348},
			script.EventMyDying:    {ashenWolf, youngBrownKeltir, brownKeltir, elderBrownKeltir},
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
	if strings.EqualFold(e.Name, "30348-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30348-00.htm"
		case p.Level() < 3:
			htmltext = "30348-01.htm"
		default:
			htmltext = "30348-02.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(darkBezoar) >= 13 {
			htmltext = "30348-05.htm"
			s.TakeItems(p, darkBezoar, -1)
			s.RewardItems(p, lesserHealingPotion, 5)
			s.RewardExpAndSp(p, 1000, 0)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30348-04.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, darkBezoar, 1, 13, chances()[e.NPC.NpcID()]) {
		st.SetCond(2)
	}
}
