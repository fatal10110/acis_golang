// Package q368 is the quest Trespassing into the Sacred Area: Restina buys
// the fangs of the Blade Tigers.
package q368

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q368_TrespassingIntoTheSacredArea"

// restina is the quest giver.
const restina = 30926

// fang is the quest item.
const fang = 5881

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Trespassing into the Sacred Area",
		QuestID: 368,
		Items:   []int32{fang},
		Bind: script.Bindings{
			script.EventQuestStart: {restina},
			script.EventTalked:     {restina},
			script.EventMyDying:    {20794, 20795, 20796, 20797},
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
	case strings.EqualFold(e.Name, "30926-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30926-05.htm"):
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
		if p.Level() < 36 {
			htmltext = "30926-01a.htm"
		} else {
			htmltext = "30926-01.htm"
		}
	case questlog.StatusStarted:
		fangs := p.ItemCount(fang)
		if fangs == 0 {
			htmltext = "30926-03.htm"
		} else {
			bonus := int32(2000)
			if fangs > 10 {
				bonus = 5730
			}
			htmltext = "30926-04.htm"
			s.TakeItems(p, 5881, -1)
			s.RewardItems(p, 57, 250*fangs+bonus)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), fang, 1, 0, chance(e.NPC.NpcID()))
}

// chance is the per-monster drop chance of the fang.
func chance(npcID int32) int32 {
	switch npcID {
	case 20794, 20796:
		return 500000
	case 20795:
		return 770000
	default:
		return 480000
	}
}
