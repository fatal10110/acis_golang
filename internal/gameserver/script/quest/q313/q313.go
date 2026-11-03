// Package q313 is the quest Collect Spores: Herbiel pays for ten spore sacs
// taken from the spore fungi.
package q313

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q313_CollectSpores"

// Item.
const sporeSac = 1118

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Collect Spores",
		QuestID: 313,
		Items:   []int32{sporeSac},
		Bind: script.Bindings{
			script.EventQuestStart: {30150}, // Herbiel
			script.EventTalked:     {30150},
			script.EventMyDying:    {20509}, // Spore Fungus
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
	if strings.EqualFold(e.Name, "30150-05.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		if p.Level() < 8 {
			htmltext = "30150-02.htm"
		} else {
			htmltext = "30150-03.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30150-06.htm"
		} else {
			htmltext = "30150-07.htm"
			s.TakeItems(p, sporeSac, -1)
			s.RewardItems(p, 57, 3500)
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
	if s.DropItems(p, sporeSac, 1, 10, 400000) {
		st.SetCond(2)
	}
}
