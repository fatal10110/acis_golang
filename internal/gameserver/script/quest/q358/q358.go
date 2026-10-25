// Package q358 is the quest Illegitimate Child of a Goddess: Grand Master
// Oltlin trades a recipe for 108 scales of the Trives and Falibati.
package q358

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q358_IllegitimateChildOfAGoddess"

// oltlin is the quest giver.
const oltlin = 30862

// scale is the quest item.
const scale = 5868

// rewards returns the recipes Oltlin picks from.
func rewards() []int32 {
	return []int32{6329, 6331, 6333, 6335, 6337, 6339, 5364, 5366}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Illegitimate Child of a Goddess",
		QuestID: 358,
		Items:   []int32{scale},
		Bind: script.Bindings{
			script.EventQuestStart: {oltlin},
			script.EventTalked:     {oltlin},
			script.EventMyDying:    {20672, 20673}, // Trives, Falibati
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
	if strings.EqualFold(e.Name, "30862-05.htm") {
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
		if p.Level() < 63 {
			htmltext = "30862-01.htm"
		} else {
			htmltext = "30862-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30862-06.htm"
		} else {
			htmltext = "30862-07.htm"
			s.TakeItems(p, scale, -1)
			list := rewards()
			s.GiveItems(p, list[s.Rnd(len(list))], 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
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
	chance := int32(660000)
	if e.NPC.NpcID() == 20672 {
		chance = 680000
	}
	if s.DropItems(p, scale, 1, 108, chance) {
		st.SetCond(2)
	}
}
