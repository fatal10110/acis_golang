// Package q116 is the quest Beyond the Hills of Winter: a dwarf brings
// Filaur bandages, energy stones and thief keys, then carries his goods to
// Obi, for soulshots or adena.
package q116

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q116_BeyondTheHillsOfWinter"

// NPCs.
const (
	filaur = 30535
	obi    = 32052
)

// Items.
const (
	bandage     = 1833
	energyStone = 5589
	thiefKey    = 1661
	goods       = 8098
)

// ssd is the soulshot reward.
const ssd = 1463

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Beyond the Hills of Winter",
		QuestID: 116,
		Items:   []int32{goods},
		Bind: script.Bindings{
			script.EventQuestStart: {filaur},
			script.EventTalked:     {filaur, obi},
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
	case strings.EqualFold(e.Name, "30535-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30535-05.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, goods, 1)
	case strings.EqualFold(e.Name, "materials"):
		htmltext = "32052-02.htm"
		s.TakeItems(p, goods, -1)
		s.RewardItems(p, ssd, 1650)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "adena"):
		htmltext = "32052-02.htm"
		s.TakeItems(p, goods, -1)
		s.GiveItems(p, 57, 16500)
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
		if p.Level() < 30 || p.Race() != player.RaceDwarf {
			htmltext = "30535-00.htm"
		} else {
			htmltext = "30535-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case filaur:
			switch cond {
			case 1:
				if p.ItemCount(bandage) >= 20 && p.ItemCount(energyStone) >= 5 && p.ItemCount(thiefKey) >= 10 {
					htmltext = "30535-03.htm"
					s.TakeItems(p, bandage, 20)
					s.TakeItems(p, energyStone, 5)
					s.TakeItems(p, thiefKey, 10)
				} else {
					htmltext = "30535-04.htm"
				}
			case 2:
				htmltext = "30535-05.htm"
			}
		case obi:
			if cond == 2 {
				htmltext = "32052-00.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}
