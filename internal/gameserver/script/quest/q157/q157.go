// Package q157 is the quest Recover Smuggled Goods: twenty adamantite ores
// taken from toads earn Wilford's buckler.
package q157

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q157_RecoverSmuggledGoods"

// Item.
const adamantiteOre = 1024

// Reward.
const buckler = 20

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Recover Smuggled Goods",
		QuestID: 157,
		Items:   []int32{adamantiteOre},
		Bind: script.Bindings{
			script.EventQuestStart: {30005}, // Wilford
			script.EventTalked:     {30005},
			script.EventMyDying:    {20121}, // Toad
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
	if strings.EqualFold(e.Name, "30005-05.htm") {
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
		if p.Level() < 5 {
			htmltext = "30005-02.htm"
		} else {
			htmltext = "30005-03.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30005-06.htm"
		case 2:
			htmltext = "30005-07.htm"
			s.TakeItems(p, adamantiteOre, -1)
			s.GiveItems(p, buckler, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
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
	if s.DropItems(p, adamantiteOre, 1, 20, 400000) {
		st.SetCond(2)
	}
}
