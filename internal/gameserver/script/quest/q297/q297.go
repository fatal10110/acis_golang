// Package q297 is the quest Gatekeeper's Favor: Wirphy gives two gatekeeper
// tokens for twenty starstones of the whinstone golems.
package q297

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q297_GatekeepersFavor"

// Items.
const (
	starstone       = 1573
	gatekeeperToken = 1659
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Gatekeeper's Favor",
		QuestID: 297,
		Items:   []int32{starstone},
		Bind: script.Bindings{
			script.EventQuestStart: {30540}, // Wirphy
			script.EventTalked:     {30540},
			script.EventMyDying:    {20521}, // Whinstone Golem
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
	if strings.EqualFold(e.Name, "30540-03.htm") {
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
		if p.Level() < 15 {
			htmltext = "30540-01.htm"
		} else {
			htmltext = "30540-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30540-04.htm"
		} else {
			htmltext = "30540-05.htm"
			s.TakeItems(p, starstone, -1)
			s.RewardItems(p, gatekeeperToken, 2)
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
	if s.DropItems(p, starstone, 1, 20, 500000) {
		st.SetCond(2)
	}
}
