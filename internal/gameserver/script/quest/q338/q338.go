// Package q338 is the quest Alligator Hunter: Enverun buys alligator pelts.
package q338

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q338_AlligatorHunter"

// Item.
const alligatorPelt = 4337

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Alligator Hunter",
		QuestID: 338,
		Items:   []int32{alligatorPelt},
		Bind: script.Bindings{
			script.EventQuestStart: {30892}, // Enverun
			script.EventTalked:     {30892},
			script.EventMyDying:    {20135}, // Alligator
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
	case strings.EqualFold(e.Name, "30892-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30892-05.htm"):
		pelts := p.ItemCount(alligatorPelt)
		reward := pelts * 60
		if pelts > 10 {
			reward += 3430
		}
		s.TakeItems(p, alligatorPelt, -1)
		s.RewardItems(p, 57, reward)
	case strings.EqualFold(e.Name, "30892-08.htm"):
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
		if p.Level() < 40 {
			htmltext = "30892-00.htm"
		} else {
			htmltext = "30892-01.htm"
		}
	case questlog.StatusStarted:
		if p.HasItem(alligatorPelt) {
			htmltext = "30892-03.htm"
		} else {
			htmltext = "30892-04.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropItemsAlways(p, alligatorPelt, 1, 0)
}
