// Package q277 is the quest Gatekeeper's Offering: Tamil trades twenty
// starstones for two gatekeeper charms.
package q277

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q277_GatekeepersOffering"

// Item.
const starstone = 1572

// Reward.
const gatekeeperCharm = 1658

// New returns the quest. Unlike most quests it names no quest items: the
// starstones are taken when the reward is paid.
func New() script.Script {
	return script.Script{
		Title:   "Gatekeeper's Offering",
		QuestID: 277,
		Bind: script.Bindings{
			script.EventQuestStart: {30576}, // Tamil
			script.EventTalked:     {30576},
			script.EventMyDying:    {20333}, // Graystone Golem
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
	if strings.EqualFold(e.Name, "30576-03.htm") {
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
			htmltext = "30576-01.htm"
		} else {
			htmltext = "30576-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30576-04.htm"
		} else {
			htmltext = "30576-05.htm"
			s.TakeItems(p, starstone, -1)
			s.RewardItems(p, gatekeeperCharm, 2)
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
