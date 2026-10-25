// Package q353 is the quest Power of Darkness: Galman buys the stones of
// darkness the Malruk succubi carry.
package q353

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q353_PowerOfDarkness"

// galman is the quest giver.
const galman = 31044

// stone is the quest item.
const stone = 5862

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Power of Darkness",
		QuestID: 353,
		Items:   []int32{stone},
		Bind: script.Bindings{
			script.EventQuestStart: {galman},
			script.EventTalked:     {galman},
			script.EventMyDying:    {20244, 20245, 20283, 20284},
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
	case strings.EqualFold(e.Name, "31044-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31044-08.htm"):
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
		if p.Level() < 55 {
			htmltext = "31044-01.htm"
		} else {
			htmltext = "31044-02.htm"
		}
	case questlog.StatusStarted:
		stones := p.ItemCount(stone)
		if stones == 0 {
			htmltext = "31044-05.htm"
		} else {
			htmltext = "31044-06.htm"
			s.TakeItems(p, stone, -1)
			s.RewardItems(p, 57, 2500+230*stones)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	chance := int32(500000)
	if npcID := e.NPC.NpcID(); npcID == 20244 || npcID == 20283 {
		chance = 480000
	}
	s.DropItems(p, stone, 1, 0, chance)
}
